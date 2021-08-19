package rtsp

import (
	"errors"
	"strconv"
	"strings"
)

type attribute struct {
	attr  string
	value string
}

func (a *attribute) parse(attrstr string) error {
	strs := strings.SplitN(attrstr, ":", 2)
	a.attr = strs[0]
	if len(strs) == 2 {
		a.value = strs[1]
	}
	return nil
}

type fmtpattr struct {
	format    int
	paramters string
}

//a=fmtp:<format> <format specific parameters>
func (f *fmtpattr) parse(fmtpstr string) error {
	strs := strings.SplitN(fmtpstr, " ", 2)
	if len(strs) == 0 {
		return errors.New("fmtp string wrong format")
	} else if len(strs) == 2 {
		f.paramters = strs[1]
	}
	f.format, _ = strconv.Atoi(strs[0])
	return nil
}

type rtpmapattr struct {
	pt         int
	encodeName string
	clockRate  int
	param      string
}

//a=rtpmap:<payload type> <encoding name>/<clock rate> [/<encoding parameters>]
func (r *rtpmapattr) parse(rtpmapstr string) error {
	strs := strings.Split(rtpmapstr, " ")
	if len(strs) < 2 {
		return errors.New("rtpmap is wring format")
	}
	r.pt, _ = strconv.Atoi(strs[0])
	encodestr := strings.Split(strs[1], "/")
	if len(encodestr) < 2 {
		return errors.New("rtpmap is wring format")
	}
	r.encodeName = encodestr[0]
	r.clockRate, _ = strconv.Atoi(encodestr[1])
	if len(encodestr) == 3 {
		r.param = encodestr[2]
	}
	return nil
}

//m=<media> <port> <proto> <fmt> ...
type mediadescription struct {
	media string
	proto string
	port  []int //for rtsp always zero
	fmt   []int
}

func (m *mediadescription) parse(mediadescribestr string) error {
	strs := strings.Split(mediadescribestr, " ")
	if len(strs) < 4 {
		return errors.New("media describe is wring format")
	}
	m.media = strs[0]
	if strings.Contains(strs[1], "/") {
		portstr := strings.Split(strs[1], "/")
		tmpport, _ := strconv.Atoi(portstr[0])
		numberofports, _ := strconv.Atoi(portstr[1])
		for i := 0; i < numberofports; i++ {
			m.port = append(m.port, tmpport+i)
		}
	} else {
		tmpport, _ := strconv.Atoi(strs[1])
		m.port = append(m.port, tmpport)
	}
	m.proto = strs[2]
	fmtnum := len(strs) - 3
	m.fmt = make([]int, fmtnum)
	for i := 0; i < fmtnum; i++ {
		m.fmt[i], _ = strconv.Atoi(strs[3+i])
	}
	return nil
}

type sdpmedia struct {
	describe   mediadescription
	rtpmap     rtpmapattr
	fmtp       fmtpattr
	controlurl string
	attrs      []attribute
}

type Rtspsdp struct {
	Medias     []sdpmedia
	Controlurl string
	Attrs      []attribute
}

func readLines(sdpstr string) []string {
	start := 0
	var result []string
	for start < len(sdpstr) {
		tmp := sdpstr[start:]
		end := strings.IndexFunc(tmp, func(c rune) bool {
			return c == '\r' || c == '\n'
		})
		if end == -1 {
			break
		} else {
			result = append(result, tmp[0:end])
			start += end + 1
			if end < len(sdpstr)-1 && sdpstr[end+1] == '\n' {
				start += 1
			}
		}
	}
	return result
}

func Parse(sdpstr string) (Rtspsdp, error) {
	lines := readLines(sdpstr)
	result := Rtspsdp{}
	mediaIdx := -1
	for i := 0; i < len(lines); i++ {
		if strings.Contains(lines[i], "m=") {
			var mediaDes sdpmedia
			err := mediaDes.describe.parse(lines[i][2:])
			if err != nil {
				return Rtspsdp{}, err
			}
			result.Medias = append(result.Medias, mediaDes)
			mediaIdx++
		} else if strings.Contains(lines[i], "a=") {
			var tmpattr attribute
			tmpattr.parse(lines[i][2:])
			switch {
			case tmpattr.attr == "control":
				if mediaIdx == -1 {
					result.Controlurl = tmpattr.value
				} else {
					result.Medias[mediaIdx].controlurl = tmpattr.value
				}
			case tmpattr.attr == "rtpmap":
				if mediaIdx == -1 {
					return Rtspsdp{}, errors.New("sdp wrong format,rtpmap before m=")
				}
				err := result.Medias[mediaIdx].rtpmap.parse(tmpattr.value)
				if err != nil {
					return Rtspsdp{}, errors.New("sdp wrong format,rtpmap parser failed")
				}
			case tmpattr.attr == "fmtp":
				if mediaIdx == -1 {
					return Rtspsdp{}, errors.New("sdp wrong format,ftmp before m=")
				}
				err := result.Medias[mediaIdx].fmtp.parse(tmpattr.value)
				if err != nil {
					return Rtspsdp{}, errors.New("sdp wrong format,ftmp parser failed")
				}
			default:
				if mediaIdx == -1 {
					result.Attrs = append(result.Attrs, tmpattr)
				} else {
					result.Medias[mediaIdx].attrs = append(result.Medias[mediaIdx].attrs, tmpattr)
				}

			}
		}
	}
	return result, nil
}
