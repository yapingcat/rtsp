package rtsp

import "errors"

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

func isKeyFrame(nalu []byte, cid Codec) bool {
	naluhdr, err := getNaluHdr(nalu)
	if err != nil {
		return false
	}

	if cid == H264 {
		nalutype := naluhdr & 0x1F
		if nalutype == 5 || nalutype == 7 || nalutype == 8 {
			return true
		}
		return false
	} else if cid == H265 {
		nalutype := (naluhdr >> 1) & 0x3F
		if (nalutype >= 16 && nalutype <= 21) || nalutype == 32 || nalutype == 33 || nalutype == 34 {
			return true
		}
		return false
	}
	return false
}
