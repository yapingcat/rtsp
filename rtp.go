package rtsp

import "errors"

// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |V=2|P|X|  CC   |M|     PT      |       sequence number         |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                           timestamp                           |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |           synchronization source (SSRC) identifier            |
// +=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+
// |            contributing source (CSRC) identifiers             |
// |                             ....                              |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
type rtphdr struct {
	version    uint8
	padding    bool
	extension  bool
	csrccount  uint8
	mark       bool
	pt         uint8
	seqnum     uint16
	timestamp  uint32
	ssrc       uint32
	csrc       []uint32
	extensions []byte
}

type rtp struct {
	head         rtphdr
	payload      []byte
	paddingcount uint8
}

func int2bool(a uint8) bool {
	if a == 0 {
		return false
	} else {
		return true
	}
}

func (r *rtp) decode(packet []byte) error {
	if len(packet) < 12 {
		return errors.New("rtp packet len < 12")
	}
	r.paddingcount = 0
	r.head.version = packet[0] & 0xC0 >> 6
	r.head.padding = int2bool(packet[0] & 0x20 >> 5)
	r.head.extension = int2bool(packet[0] & 0x10 >> 4)
	r.head.csrccount = packet[0] & 0x0F
	r.head.mark = int2bool(packet[1] & 0x80 >> 7)
	r.head.pt = packet[1] & 0x7F
	r.head.seqnum = uint16(packet[2])<<8 | uint16(packet[3])
	r.head.timestamp = uint32(packet[4])<<24 | uint32(packet[5])<<16 | uint32(packet[6])<<8 | uint32(packet[7])
	r.head.ssrc = uint32(packet[8])<<24 | uint32(packet[9])<<16 | uint32(packet[10])<<8 | uint32(packet[11])
	headlen := 12

	if len(packet) < headlen+int(r.head.csrccount)*4 {
		return errors.New("have no enough space to storage csrc")
	}
	r.head.csrc = make([]uint32, r.head.csrccount)
	for i := 0; i < int(r.head.csrccount); i++ {
		r.head.csrc[i] = uint32(packet[12+i*4])<<24 | uint32(packet[13+i*4])<<16 | uint32(packet[14+i*4])<<8 | uint32(packet[15+i*4])
	}
	headlen += int(r.head.csrccount) * 4
	if r.head.extension {
		if len(packet) < 13 {
			return errors.New("no extensions")
		}
		count := packet[12] & 0x0F
		if len(packet) < 4*int(count)+13 {
			return errors.New("has no enough bytes")
		}
		r.head.extensions = packet[13 : 13+count*4]
		headlen += int(count)*4 + 1
	}
	if r.head.padding {
		r.paddingcount = packet[len(packet)-1]
	}
	r.payload = packet[headlen : len(packet)-int(r.paddingcount)]
	return nil
}
