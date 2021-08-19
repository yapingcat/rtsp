package rtsp

import (
	"bytes"
	"errors"
	"fmt"
)

type RtpProfile int

const (
	RTP_H264 RtpProfile = 96
	RTP_H265            = 97
)

type payload interface {
	decode([]byte) error
	encode([]byte) error
	setOnPacket(onpacket func(data []byte, timestamp uint32))
}

type h264RtpPayload struct {
	cache_ bytes.Buffer
	//if current rtp pakcet of frame has been losted, different timestamp means different frame
	//lastTimestamp help to split frame
	onPacket func(data []byte, timestamp uint32)
}

func newH264Payload() *h264RtpPayload {
	h264payload := new(h264RtpPayload)
	h264payload.cache_.Write([]byte{0x00, 0x00, 0x00, 0x01})
	return h264payload
}

type h265RtpPayload struct {
	cache_   bytes.Buffer
	onPacket func(data []byte, timestamp uint32)
}

func newH265Payload() *h265RtpPayload {
	h265payload := new(h265RtpPayload)
	h265payload.cache_.Write([]byte{0x00, 0x00, 0x00, 0x01})
	return h265payload
}

func createRtpPayload(profile RtpProfile) (payload, error) {
	switch {
	case profile == RTP_H264:
		return newH264Payload(), nil
	case profile == RTP_H265:
		return newH265Payload(), nil
	default:
		return nil, errors.New("unsupport rtp profile")
	}
}

func createRtpPayloadByName(name string) (payload, error) {
	switch {
	case name == "H264":
		return newH264Payload(), nil
	case name == "H265":
		return newH265Payload(), nil
	default:
		return nil, errors.New("unsupport rtp profile")
	}
}

func (h264 *h264RtpPayload) decode(packet []byte) error {
	var rtppacket rtp
	err := rtppacket.decode(packet)
	if err != nil {
		return err
	}

	payloadType := rtppacket.payload[0] & 0x1F
	switch {
	case payloadType >= 1 && payloadType <= 23:
		h264.cache_.Write(rtppacket.payload)
		if h264.onPacket != nil {
			h264.onPacket(h264.cache_.Bytes(), rtppacket.head.timestamp)
		}
		h264.cache_.Truncate(4)
	case payloadType == 28:
		return h264.decodeFu(rtppacket.payload, rtppacket.head.timestamp, false)
	case payloadType == 29:
		return h264.decodeFu(rtppacket.payload, rtppacket.head.timestamp, true)
	default:
		return errors.New("unsupport packet type")
	}
	return nil
}

// +---------------+
// |0|1|2|3|4|5|6|7|
// +-+-+-+-+-+-+-+-+
// |S|E|R|  Type   |
// +---------------+
func (h264 *h264RtpPayload) decodeFu(packet []byte, timestamp uint32, fu_b bool) error {
	fuheader := packet[1]
	var prefixLen int = 0
	if fu_b {
		prefixLen = 4
	} else {
		prefixLen = 2
	}
	startbit := int2bool(fuheader & 0x80)
	endbit := int2bool(fuheader & 0x40)
	if startbit {
		if h264.cache_.Len() > 4 {
			fmt.Println("somthing wrong happend maybe packet lost,discard dirty frame")
			h264.cache_.Truncate(4)
		}
		h264.cache_.WriteByte((packet[0] & 0xE0) | (packet[1] & 0x1F))
	}
	h264.cache_.Write(packet[prefixLen:])
	//fmt.Printf("cache buf len %d\n", h264.cache_.Len())
	if endbit {
		if h264.onPacket != nil {
			h264.onPacket(h264.cache_.Bytes(), timestamp)
		}
		h264.cache_.Truncate(4)
	}
	return nil
}

func (h264 *h264RtpPayload) encode([]byte) error {
	return nil
}

func (h264 *h264RtpPayload) setOnPacket(onpacket func(data []byte, timestamp uint32)) {
	h264.onPacket = onpacket
}

func (h265 *h265RtpPayload) decode(packet []byte) error {
	var rtppacket rtp
	err := rtppacket.decode(packet)
	if err != nil {
		return err
	}

	payloadType := rtppacket.payload[0] >> 1 & 0x3F
	//fmt.Printf("payload type %d\n", payloadType)
	switch payloadType {
	case 48:
		return h265.decodeAP(rtppacket.payload, rtppacket.head.timestamp)
	case 49:
		return h265.decodeFu(rtppacket.payload, rtppacket.head.timestamp)
	case 50:
		return h265.decodePACI(rtppacket.payload, rtppacket.head.timestamp)
	default:
		h265.cache_.Write(rtppacket.payload)
		if h265.onPacket != nil {
			h265.onPacket(h265.cache_.Bytes(), rtppacket.head.timestamp)
		}
		h265.cache_.Truncate(4)
	}
	return nil
}

func (h265 *h265RtpPayload) decodeAP(packet []byte, timestamp uint32) error {
	return nil
}

// +---------------+
// |0|1|2|3|4|5|6|7|
// +-+-+-+-+-+-+-+-+
// |S|E|  FuType   |
// +---------------+
func (h265 *h265RtpPayload) decodeFu(packet []byte, timestamp uint32) error {
	fuheader := packet[2]
	var prefixLen int = 0
	prefixLen = 3
	startbit := int2bool(fuheader & 0x80)
	endbit := int2bool(fuheader & 0x40)
	if startbit {
		if h265.cache_.Len() > 4 {
			fmt.Println("somthing wrong happend maybe packet lost,discard dirty frame")
			h265.cache_.Truncate(4)
		}

		h265.cache_.WriteByte((packet[0] & 0x81) | ((packet[2] & 0x3F) << 1))
		h265.cache_.WriteByte(packet[1])
	}

	h265.cache_.Write(packet[prefixLen:])
	//fmt.Printf("cache buf len %d\n", h264.cache_.Len())
	if endbit {
		if h265.onPacket != nil {
			h265.onPacket(h265.cache_.Bytes(), timestamp)
		}
		h265.cache_.Truncate(4)
	}
	return nil
}

func (h265 *h265RtpPayload) decodePACI(packet []byte, timestamp uint32) error {
	return nil
}

func (h265 *h265RtpPayload) encode([]byte) error {
	return nil
}

func (h265 *h265RtpPayload) setOnPacket(onpacket func(data []byte, timestamp uint32)) {
	h265.onPacket = onpacket
}
