package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/nareix/joy5/av"
	"github.com/nareix/joy5/codec/aac"
	"github.com/nareix/joy5/codec/h264"
	"github.com/nareix/joy5/format/rtmp"
	"github.com/notedit/media-server-go"
	rtmppusher "github.com/notedit/media-server-go-demo/rtmp-to-webrtc2/rtmp"
	"github.com/notedit/sdp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v2"
	"os"
	"strings"
	"time"
)

const (
	DefaultOpusSSRC = 111111111
	DefaultH264SSRC = 333333333
)

var audioCapability = &sdp.Capability{
	Codecs: []string{"opus"},
}

var videoCapability = &sdp.Capability{
	Codecs: []string{"h264"},
}

var NALUHeader = []byte{0, 0, 0, 1}

type Stream struct {
	conn av.PacketReader
	buf  av.Packet

	audio *mediaserver.MediaFrameSession
	video *mediaserver.MediaFrameSession

	videoPacketizer rtp.Packetizer
	audioPacketizer rtp.Packetizer

	lastVideoTime time.Duration
	lastAudioTime time.Duration

	dumper *os.File
}

// NewMediaTransform  create media transform
func NewStreamer(ctx context.Context, conn av.PacketReader) *Stream {
	streamer := &Stream{}
	streamer.conn = conn

	audioMedia := sdp.MediaInfoCreate("audio", audioCapability)
	videoMedia := sdp.MediaInfoCreate("video", videoCapability)

	videoSession := mediaserver.NewMediaFrameSession(videoMedia)
	audioSession := mediaserver.NewMediaFrameSession(audioMedia)

	streamer.video = videoSession
	streamer.audio = audioSession

	audioPt := uint8(audioMedia.GetCodec("opus").GetType())
	videoPt := uint8(videoMedia.GetCodec("h264").GetType())

	videoCodec := webrtc.NewRTPH264Codec(videoPt, 90000)
	audioCodec := webrtc.NewRTPOpusCodec(audioPt, 48000)

	videoPacketizer := rtp.NewPacketizer(
		1200,
		videoCodec.PayloadType,
		DefaultH264SSRC,
		videoCodec.Payloader,
		rtp.NewFixedSequencer(1),
		videoCodec.ClockRate,
	)

	audioPacketizer := rtp.NewPacketizer(
		1200,
		audioCodec.PayloadType,
		DefaultOpusSSRC,
		audioCodec.Payloader,
		rtp.NewFixedSequencer(1),
		audioCodec.ClockRate,
	)

	streamer.videoPacketizer = videoPacketizer
	streamer.audioPacketizer = audioPacketizer

	streamer.dumper, _ = os.Create("./test.h264")

	go streamer.readLoop(ctx)
	return streamer
}

func (self *Stream) GetVideoTrack() *mediaserver.IncomingStreamTrack {

	if self.video != nil {
		return self.video.GetIncomingStreamTrack()
	}
	return nil
}

func (self *Stream) GetAuidoTrack() *mediaserver.IncomingStreamTrack {

	if self.audio != nil {
		return self.audio.GetIncomingStreamTrack()
	}
	return nil
}

func (self *Stream) readLoop(ctx context.Context) {
	s := strings.Split(self.conn.(*rtmp.Conn).URL.Path, "/")
	pusher, err := rtmppusher.NewRtmpPusher(s[2])
	if err != nil {
		fmt.Println(err)
	}
	adtsheader := make([]byte, 7)
	pusher.Start()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		pkt, err := self.conn.ReadPacket()
		if err != nil {
			return
		}

		switch pkt.Type {
		case av.H264DecoderConfig:
			self.buf.VSeqHdr = append([]byte(nil), pkt.Data...)
			self.buf.H264, _ = h264.FromDecoderConfig(self.buf.VSeqHdr)

			// dumper
			self.dumper.Write(NALUHeader)
			self.dumper.Write(self.buf.H264.SPS[0])
			self.dumper.Write(NALUHeader)
			self.dumper.Write(self.buf.H264.PPS[0])

			pusher.Push(NALUHeader, false)
			pusher.Push(self.buf.H264.SPS[0], false)
			pusher.Push(NALUHeader, false)
			pusher.Push(self.buf.H264.PPS[0], false)

		case av.H264:
			pkt.Metadata = self.buf.Metadata
			if pkt.IsKeyFrame {
				pkt.VSeqHdr = self.buf.VSeqHdr
			}

			var samples uint32
			if self.lastVideoTime == 0 {
				samples = 0
			} else {
				samples = uint32(uint64(pkt.Time-self.lastVideoTime) * 90000 / 1000000000)
			}

			var b bytes.Buffer
			if pkt.IsKeyFrame {
				b.Write(NALUHeader)
				b.Write(self.buf.H264.SPS[0])
				b.Write(NALUHeader)
				b.Write(self.buf.H264.PPS[0])
			}

			nalus, _ := h264.SplitNALUs(pkt.Data)
			nalusbuf := h264.JoinNALUsAnnexb(nalus)
			b.Write(nalusbuf)

			self.dumper.Write(nalusbuf)
			pusher.Push(nalusbuf, false)

			pkts := self.videoPacketizer.Packetize(b.Bytes(), samples)

			for _, rtppkt := range pkts {
				buf, _ := rtppkt.Marshal()
				self.video.Push(buf)
				//pusher.Push(buf, false)
			}
			self.lastVideoTime = pkt.Time

		case av.AACDecoderConfig:
			self.buf.ASeqHdr = append([]byte(nil), pkt.Data...)
			self.buf.AAC, _ = aac.FromMPEG4AudioConfigBytes(pkt.Data)
			fmt.Println(self.buf.AAC)
			pusher.Push(NALUHeader, true)
			pusher.Push(self.buf.AAC.ConfigBytes, true)
		case av.AAC:
			pkt.Metadata = self.buf.Metadata
			pkt.ASeqHdr = self.buf.ASeqHdr
			//pusher.Push(pkt.Data, true)
			adtsbuffer := []byte{}
			aac.FillADTSHeader(adtsheader, self.buf.AAC.Config, 1024, len(pkt.Data))
			adtsbuffer = append(adtsbuffer, adtsheader...)
			adtsbuffer = append(adtsbuffer, pkt.Data...)
			pusher.Push(adtsbuffer, true)
			// todo
		case av.Metadata:
			self.buf.Metadata = pkt.Data
		}

	}

}

func (self *Stream) Close() error {

	return nil
}
