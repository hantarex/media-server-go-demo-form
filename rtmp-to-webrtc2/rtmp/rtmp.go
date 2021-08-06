package rtmp2

import (
	"fmt"

	"github.com/notedit/gst"
)

//const pipelinestring = "appsrc is-live=true do-timestamp=true name=videosrc ! h264parse ! video/x-h264,stream-format=(string)avc ! muxer.   appsrc is-live=true do-timestamp=true name=audiosrc ! opusparse ! opusdec ! audioconvert ! audioresample ! faac ! muxer.  flvmux name=muxer ! rtmpsink sync=false location='%s live=1'"
//const pipelinestring = "appsrc is-live=true do-timestamp=true name=videosrc ! h264parse ! video/x-h264,stream-format=(string)avc ! flvmux ! filesink location=xyz.flv"
const pipelinestring = "appsrc is-live=true do-timestamp=true name=videosrc ! queue2 ! h264parse ! nvh264dec ! videoscale ! video/x-raw, width=2560, height=1440 ! nvh264enc ! h264parse ! queue2 ! hlssink2 playlist-length=5 target-duration=5 location=%s/segment%%05d.ts playlist-location=%s/playlist.m3u8"

type RtmpPusher struct {
	pipeline *gst.Pipeline
	videosrc *gst.Element
	audiosrc *gst.Element
}

func NewRtmpPusher(rtmpUrl string) (*RtmpPusher, error) {

	err := gst.CheckPlugins([]string{"flv", "rtmp"})

	if err != nil {
		return nil, err
	}

	pipelineStr := fmt.Sprintf(pipelinestring, rtmpUrl, rtmpUrl)

	fmt.Println(pipelineStr)

	pipeline, err := gst.ParseLaunch(pipelineStr)

	if err != nil {
		return nil, err
	}

	videosrc := pipeline.GetByName("videosrc")
	//audiosrc := pipeline.GetByName("audiosrc")

	pusher := &RtmpPusher{
		pipeline: pipeline,
		videosrc: videosrc,
		//audiosrc: audiosrc,
	}

	return pusher, nil
}

func (p *RtmpPusher) Start() {

	p.pipeline.SetState(gst.StatePlaying)
}

func (p *RtmpPusher) Stop() {

	p.pipeline.SetState(gst.StateNull)
}

func (p *RtmpPusher) Push(buffer []byte, audio bool) {
	var err error
	if audio {
		err = p.audiosrc.PushBuffer(buffer)
	} else {
		err = p.videosrc.PushBuffer(buffer)
	}

	if err != nil {
		fmt.Println("push buffer error", err)
	}
}
