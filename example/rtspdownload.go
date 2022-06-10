package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/yapingcat/rtsp"
)

func getNaluHdr(nalu []byte) (uint8, error) {
	if nalu[0] == 0x00 && nalu[1] == 0x00 {
		if nalu[2] == 0x01 {
			return nalu[3], nil
		} else if nalu[2] == 0x00 && nalu[3] == 0x01 {
			return nalu[4], nil
		}

	}
	return 0, errors.New("Illegal Nalu")
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: ", os.Args[0], " rtspurl ", "videofile")
		return
	}
	url := os.Args[1]
	h264file := os.Args[2]
	client := rtsp.BuildRtspClient(url)
	os.Create(h264file)
	f, err := os.OpenFile(h264file, os.O_RDWR, 0666)
	if err != nil {
		fmt.Println(err)
	}

	client.OnFrame = func(frame rtsp.Frame) {
		var frametype string
		var codectype string
		var slicetype string

		frames := bytes.Split(frame.Data, []byte{0x00, 0x00, 0x00, 0x01})
		for i := 0; i < len(frames); i++ {
			if len(frames[i]) == 0 {
				continue
			}
			if frame.Cid == rtsp.H264 {
				nalutype := frames[i][0] & 0x1F
				frametype = "video"
				codectype = "H264"
				switch nalutype {
				case 5:
					slicetype = "I"
				case 7:
					slicetype = "SPS"
				case 8:
					slicetype = "PPS"
				case 1:
					slicetype = "P/B"
				case 6:
					slicetype = "SEI"
				case 9:
					slicetype = "AUD"
				}
			} else if frame.Cid == rtsp.H265 {
				frametype = "video"
				codectype = "H265"
				nalutype := (frames[i][0] >> 1) & 0x3F
				switch {
				case nalutype >= 16 && nalutype <= 21:
					slicetype = "I"
				case nalutype == 32:
					slicetype = "VPS"
				case nalutype == 33:
					slicetype = "SPS"
				case nalutype == 34:
					slicetype = "PPS"
				case nalutype == 39:
					slicetype = "SEI"
				case nalutype == 40:
					slicetype = "SEI"
				case nalutype >= 0 && nalutype <= 15:
					slicetype = "P/B"
				case nalutype >= 22 && nalutype <= 31:
					slicetype = "P/B"
				case nalutype == 35:
					slicetype = "AUD"
				}
			}
			fmt.Printf("[%s][%s] [ts=%-8d] [len=%-6d] [%-3s]\n", frametype, codectype, frame.Ts, len(frames[i]), slicetype)
		}

		f.Write(frame.Data)
	}
	client.Start()
	select {}
}
