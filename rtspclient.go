package rtsp

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Codec int

const (
	UNSupport Codec = iota
	H264      Codec = iota
	H265
	AAC
	G711A
	G711U
)

type Frame struct {
	Cid   Codec
	Data  []byte
	Ts    uint32
	IsKey bool
}

type HandleRtspMethod interface {
	handleOption(res Response) error
	handleDescribe(res Response) error
	handleSetup(res Response) error
	handlePlay(res Response) error
	handleTearDown(res Response) error
	handleReponse(res Response) error
	handleUnauthorized(res Response) error
}

type meidaTransport struct {
	uri         string
	RtpChannel  int
	RtcpChannel int
	rtpdecoder  payload
}

type Rtspclient struct {
	url           string
	username      string
	password      string
	host          string
	conn          net.Conn
	recvBuf       *bytes.Buffer
	mediaChanel   []meidaTransport
	stopFlag      bool
	setupStep     int
	cseq          int
	session       string
	sdp           Rtspsdp
	aliveTimeout  int
	vcid          Codec
	acid          Codec
	secure        bool
	sps           []byte
	pps           []byte
	vps           []byte
	handleReponse func(res Response) error
	OnFrame       func(frame Frame)
	auth          DigestAuthenticate
	needAuth      bool
	keepAlive     bool
	aliveTicker   *time.Ticker
}

func (c *Rtspclient) handleOption(res Response) error {

	if res.StatusCode == "401" {
		return c.handleUnauthorized("OPTIONS", res)
	}
	if c.keepAlive {
		return nil
	}
	_, ok := res.HeaderFileds["Public"]
	if !ok {
		fmt.Println("WARNING,has no Public Filed")
	}

	if c.setupStep == 0 { //start to create rtsp session
		describeReq := MakeDescribe(c.url)
		describeReq.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
		c.cseq++
		c.handleReponse = c.handleDescribe
		c.auth.uri = c.url
		c.auth.method = "DESCRIBE"
		if c.needAuth {
			authinfo := c.auth.digestInfo()
			describeReq.HeaderFileds["Authorization"] = authinfo
		}
		return c.sendRtspCommad([]byte(describeReq.ToString()))
	} else { //for keepalive
		//do something ,but now,i don't konw
		fmt.Println("keepalive options response")
		return nil
	}
}

func (c *Rtspclient) handleDescribe(res Response) error {

	if res.StatusCode == "401" {
		return c.handleUnauthorized("DESCRIBE", res)
	}

	var err error
	c.sdp, err = Parse(string(res.Body))
	if err != nil {
		return err
	}
	if len(c.sdp.Medias) == 0 {
		return errors.New("has no media describe")
	}

	// 1.     The RTSP Content-Base field
	// 2.     The RTSP Content-Location field
	// 3.     The RTSP request URL
	baseurl, exist := res.HeaderFileds["Content-Base"]
	if !exist {
		locationurl, exist := res.HeaderFileds["Content-Location"]
		if !exist {
			baseurl = c.url
		} else {
			baseurl = locationurl
		}
	}
	if !strings.HasSuffix(baseurl, "/") {
		baseurl += "/"
	}

	for i := 0; i < len(c.sdp.Medias); i++ {
		var mediaTrans meidaTransport
		mediaTrans.RtcpChannel = -1
		mediaTrans.RtpChannel = -1
		mediaTrans.rtpdecoder, _ = createRtpPayloadByName(c.sdp.Medias[i].rtpmap.encodeName)
		if c.sdp.Medias[i].describe.media == "video" {
			if c.sdp.Medias[i].rtpmap.encodeName == "H264" {
				c.vcid = H264

				params := strings.Split(c.sdp.Medias[i].fmtp.paramters, ";")
				for i := 0; i < len(params); i++ {
					if strings.Contains(params[i], "sprop-parameter-sets") {
						spropParameterSets := strings.TrimSpace(params[i])
						spspps := strings.Split(strings.TrimPrefix(spropParameterSets, "sprop-parameter-sets="), ",")
						spsbase64 := spspps[0]
						ppsbase64 := spspps[1]
						fmt.Println("get sps from sdp")
						fmt.Println(params[i])
						fmt.Println(spspps)
						c.sps, _ = base64.StdEncoding.DecodeString(spsbase64)
						c.sps = append([]byte{0x00, 0x00, 0x00, 0x01}, c.sps...)
						c.pps, _ = base64.StdEncoding.DecodeString(ppsbase64)
						c.pps = append([]byte{0x00, 0x00, 0x00, 0x01}, c.pps...)
					}
				}
			} else if c.sdp.Medias[i].rtpmap.encodeName == "H265" {
				params := strings.Split(c.sdp.Medias[i].fmtp.paramters, ";")
				for i := 0; i < len(params); i++ {
					if strings.Contains(params[i], "sprop-vps") {
						vpsbase64 := strings.TrimPrefix(params[i], "sprop-vps=")
						c.vps, _ = base64.StdEncoding.DecodeString(vpsbase64)
						c.vps = append([]byte{0x00, 0x00, 0x00, 0x01}, c.vps...)
					} else if strings.Contains(params[i], "sprop-sps") {
						spsbase64 := strings.TrimSpace(params[i])
						spsbase64 = strings.TrimPrefix(spsbase64, "sprop-sps=")
						c.sps, _ = base64.StdEncoding.DecodeString(spsbase64)
						c.sps = append([]byte{0x00, 0x00, 0x00, 0x01}, c.sps...)
					} else if strings.Contains(params[i], "sprop-pps") {
						ppsbase64 := strings.TrimSpace(params[i])
						ppsbase64 = strings.TrimPrefix(ppsbase64, "sprop-pps=")
						c.pps, _ = base64.StdEncoding.DecodeString(ppsbase64)
						c.pps = append([]byte{0x00, 0x00, 0x00, 0x01}, c.pps...)
					}
				}
				c.vcid = H265
			} else {
				return errors.New("UnSupport Video Codec")
			}
			if mediaTrans.rtpdecoder != nil {
				mediaTrans.rtpdecoder.setOnPacket(c.onVideo)
			}
		} else if c.sdp.Medias[i].describe.media == "audio" {
			if c.sdp.Medias[i].rtpmap.encodeName == "PCMA" {
				c.acid = G711A
			} else if c.sdp.Medias[i].rtpmap.encodeName == "PCMU" {
				c.acid = G711U
			} else if c.sdp.Medias[i].rtpmap.encodeName == "mpeg4-generic" || c.sdp.Medias[i].rtpmap.encodeName == "MPEG4-GENERIC" {
				c.acid = AAC
			} else {
				continue
			}
			if mediaTrans.rtpdecoder != nil {
				mediaTrans.rtpdecoder.setOnPacket(c.onAudio)
			}
		} else {
			continue
		}

		var absoluteUrl string
		if strings.HasPrefix(c.sdp.Medias[i].controlurl, "rtsp://") {
			absoluteUrl = c.sdp.Medias[i].controlurl
		} else if c.sdp.Medias[i].controlurl == "*" {
			absoluteUrl = baseurl
		} else {
			if strings.HasPrefix(c.sdp.Medias[i].controlurl, "/") {
				absoluteUrl = baseurl + c.sdp.Medias[i].controlurl[1:]
			} else {
				absoluteUrl = baseurl + c.sdp.Medias[i].controlurl
			}
		}
		mediaTrans.uri = absoluteUrl
		c.mediaChanel = append(c.mediaChanel, mediaTrans)
	}
	fmt.Println(len(c.mediaChanel))
	req := MakeSetup(c.mediaChanel[c.setupStep].uri)
	c.auth.method = "SETUP"
	c.auth.uri = c.mediaChanel[c.setupStep].uri
	if c.needAuth {
		authinfo := c.auth.digestInfo()
		req.HeaderFileds["Authorization"] = authinfo
	}
	req.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
	tcptransport := TcpTransport{Mode: "PLAY", Interleaved: [2]int{c.setupStep * 2, c.setupStep*2 + 1}}
	req.HeaderFileds["Transport"] = tcptransport.ToString()
	c.cseq++
	c.handleReponse = c.handleSetup
	return c.sendRtspCommad([]byte(req.ToString()))
}

func (c *Rtspclient) handleSetup(res Response) error {

	if res.StatusCode == "401" {
		return c.handleUnauthorized("SETUP", res)
	}
	trans, ok := res.HeaderFileds["Transport"]
	if !ok {
		return errors.New("response has no Transport")
	}
	sessionid, ok := res.HeaderFileds["Session"]
	if !ok {
		return errors.New("response has no Session")
	}
	timeoutIdx := strings.Index(sessionid, "timeout=")
	if timeoutIdx > 0 {
		timeout, _ := strconv.Atoi(strings.TrimSpace(sessionid[timeoutIdx+8:]))
		c.session = sessionid[:timeoutIdx]
		c.aliveTimeout = timeout
	} else {
		c.session = sessionid
	}

	var tcptrans TcpTransport
	tcptrans.Parser(trans)
	c.mediaChanel[c.setupStep].RtpChannel = tcptrans.Interleaved[0]
	c.mediaChanel[c.setupStep].RtcpChannel = tcptrans.Interleaved[1]
	c.setupStep++

	if c.setupStep >= len(c.mediaChanel) {
		playreq := MakePlay(c.url)
		playreq.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
		playreq.HeaderFileds["Session"] = c.session
		c.cseq++
		c.handleReponse = c.handlePlay
		c.auth.uri = c.url
		c.auth.method = "PLAY"
		if c.needAuth {
			authinfo := c.auth.digestInfo()
			playreq.HeaderFileds["Authorization"] = authinfo
		}
		return c.sendRtspCommad([]byte(playreq.ToString()))
	} else {
		setupreq := MakeSetup(c.mediaChanel[c.setupStep].uri)
		setupreq.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
		setupreq.HeaderFileds["Session"] = c.session
		tcptransport := TcpTransport{Mode: "PLAY", Interleaved: [2]int{c.setupStep * 2, c.setupStep*2 + 1}}
		setupreq.HeaderFileds["Transport"] = tcptransport.ToString()
		c.cseq++
		c.handleReponse = c.handleSetup
		c.auth.uri = c.mediaChanel[c.setupStep].uri
		c.auth.method = "SETUP"
		if c.needAuth {
			authinfo := c.auth.digestInfo()
			setupreq.HeaderFileds["Authorization"] = authinfo
		}
		return c.sendRtspCommad([]byte(setupreq.ToString()))
	}
}

func (c *Rtspclient) handlePlay(res Response) error {
	if res.StatusCode == "200" {
		c.keepAlive = true
		go func() {
			fmt.Println("alive time out ", c.aliveTimeout)
			c.aliveTicker = time.NewTicker(time.Second * time.Duration(c.aliveTimeout/2))
			defer c.aliveTicker.Stop()
			cc := c.aliveTicker.C
			for !c.stopFlag {
				<-cc
				if c.stopFlag {
					continue
				}
				option := MakeOption(c.url)
				option.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
				option.HeaderFileds["Session"] = c.session
				c.cseq++
				c.handleReponse = c.handleOption
				c.auth.uri = c.url
				c.auth.method = "OPTIONS"
				if c.needAuth {
					authinfo := c.auth.digestInfo()
					option.HeaderFileds["Authorization"] = authinfo
				}
				if err := c.sendRtspCommad([]byte(option.ToString())); err != nil {
					fmt.Println("send KeepAlive Command Failed ", err)
				}
			}
		}()
		fmt.Println("play ok")
	} else {
		fmt.Println(res.StatusCode)
	}
	return nil
}

func (c *Rtspclient) handleTearDown(res Response) error {
	c.Stop()
	return nil
}

func (c *Rtspclient) handleUnauthorized(method string, res Response) error {
	c.needAuth = true
	authstr, ok := res.HeaderFileds["WWW-Authenticate"]
	if !ok {
		return errors.New("has no fileds WWW-Authenticate")
	}
	c.auth.method = method
	var authinfo string
	authstr = strings.TrimSpace(authstr)
	if strings.HasPrefix(authstr, "Digest") {
		c.auth.parse(strings.TrimPrefix(authstr, "Digest"))
		authinfo = c.auth.digestInfo()
	} else {
		return errors.New("Unsupport auth")
	}

	var req Request
	switch method {
	case "OPTIONS":
		req = MakeOption(c.auth.uri)
	case "DESCRIBE":
		req = MakeDescribe(c.auth.uri)
	case "SETUP":
		req = MakeSetup(c.auth.uri)
	case "PLAY":
		req = MakePlay(c.auth.uri)
	}
	req.HeaderFileds["Authorization"] = authinfo
	req.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
	if c.session != "" {
		req.HeaderFileds["Session"] = c.session
	}
	c.cseq++
	return c.sendRtspCommad([]byte(req.ToString()))
}

func (c *Rtspclient) onVideo(videoData []byte, timestamp uint32) {

	naluhdr, err := getNaluHdr(videoData)
	if err != nil {
		return
	}
	var videoFrame Frame
	if c.vcid == H264 {
		nalutype := naluhdr & 0x1F
		switch nalutype {
		case 5:
			frame := append(c.sps, c.pps...)
			frame = append(frame, videoData...)
			videoFrame = Frame{Cid: c.vcid, Data: frame, Ts: timestamp, IsKey: true}
		case 7:
			if !bytes.Equal(c.sps, videoData) {
				c.sps = make([]byte, len(videoData))
				copy(c.sps, videoData)
			}
		case 8:
			if !bytes.Equal(c.pps, videoData) {
				c.pps = make([]byte, len(videoData))
				copy(c.pps, videoData)
			}
		default:
			videoFrame = Frame{Cid: c.vcid, Data: videoData, Ts: timestamp, IsKey: false}
		}
	} else if c.vcid == H265 {
		nalutype := (naluhdr >> 1) & 0x3F
		switch {
		case (nalutype >= 16 && nalutype <= 21):
			frame := append(c.vps, c.sps...)
			frame = append(frame, c.pps...)
			frame = append(frame, videoData...)
			videoFrame = Frame{Cid: c.vcid, Data: frame, Ts: timestamp, IsKey: true}
		case nalutype == 32:
			if !bytes.Equal(c.vps, videoData) {
				c.vps = make([]byte, len(videoData))
				copy(c.vps, videoData)
			}
		case nalutype == 33:
			if !bytes.Equal(c.sps, videoData) {
				c.sps = make([]byte, len(videoData))
				copy(c.sps, videoData)
			}
		case nalutype == 34:
			if !bytes.Equal(c.pps, videoData) {
				c.pps = make([]byte, len(videoData))
				copy(c.pps, videoData)
			}
		default:
			videoFrame = Frame{Cid: c.vcid, Data: videoData, Ts: timestamp, IsKey: false}
		}
	}
	if c.OnFrame != nil {
		c.OnFrame(videoFrame)
	}
}

func (c *Rtspclient) onAudio(audioData []byte, timestamp uint32) {
	videoFrame := Frame{Cid: c.vcid, Data: audioData, Ts: timestamp, IsKey: true}
	if c.OnFrame != nil {
		c.OnFrame(videoFrame)
	}
}

func BuildRtspClient(rtspurl string) *Rtspclient {
	client := new(Rtspclient)
	client.url = rtspurl
	tmpurl, err := url.Parse(rtspurl)
	if err != nil {
		return nil
	}
	if strings.ToLower(tmpurl.Scheme) == "rtsps" {
		client.secure = true
	}
	client.host = tmpurl.Host
	if tmpurl.Port() == "" {
		client.host += ":554"
	}
	if tmpurl.User != nil {
		client.username = tmpurl.User.Username()
		client.password, _ = tmpurl.User.Password()
	}
	tmpurl.User = nil
	client.url = tmpurl.String()
	client.aliveTimeout = 60
	client.auth.password = client.password
	client.auth.username = client.username
	client.needAuth = false
	client.keepAlive = false
	return client
}

func (c *Rtspclient) Start() {
	if c.secure {
		fmt.Println("start rtsps")
		conf := &tls.Config{
			InsecureSkipVerify: true,
		}
		conn, err := tls.Dial("tcp", c.host, conf)
		if err != nil {
			log.Println(err)
			return
		}
		c.conn = conn
	} else {
		conn, err := net.DialTimeout("tcp", c.host, time.Second*5)
		if err != nil {
			fmt.Println("connect failed " + err.Error())
			return
		}
		c.conn = conn
	}

	c.recvBuf = new(bytes.Buffer)
	c.stopFlag = false
	c.handleReponse = c.handleOption
	c.cseq = 1
	c.session = ""
	req := MakeOption(c.url)
	req.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
	c.cseq++
	c.auth.uri = c.url
	err := c.sendRtspCommad([]byte(req.ToString()))
	if err != nil {
		return
	}
	go c.cycleRecv()
}

func (c *Rtspclient) Stop() {
	if !c.stopFlag {
		c.stopFlag = true
		c.aliveTicker.Reset(time.Millisecond * 10)
		tearDown := MakeTearDown(c.url)
		tearDown.HeaderFileds["CSeq"] = strconv.Itoa(c.cseq)
		c.cseq++
		c.auth.uri = c.url
		c.auth.method = "TEARDOWN"
		if c.needAuth {
			authinfo := c.auth.digestInfo()
			tearDown.HeaderFileds["Authorization"] = authinfo
		}
		c.sendRtspCommad([]byte(tearDown.ToString()))
		c.conn.Close()
	}
}

func (c *Rtspclient) cycleRecv() {
	defer c.Stop()
	for !c.stopFlag {
		buf := make([]byte, 4096)
		readLen, err := c.conn.Read(buf)
		if err != nil {
			fmt.Println(err)
			return
		}
		c.recvBuf.Write(buf[:readLen])
		var needMore bool = false
		for c.recvBuf.Len() > 0 && !needMore {
			if c.recvBuf.Bytes()[0] == '$' {
				needMore, err = c.rtpOverRtsp()
			} else {
				needMore, err = c.handleRtspMessage()
			}
			if err != nil {
				fmt.Println(err)
				return
			}
		}
	}
}

func (c *Rtspclient) rtpOverRtsp() (bool, error) {
	if c.recvBuf.Len() < 4 {
		return true, nil
	}
	channel := c.recvBuf.Bytes()[1]
	rtppacketlen := uint16(c.recvBuf.Bytes()[2])<<8 | uint16(c.recvBuf.Bytes()[3])
	if c.recvBuf.Len() < int(rtppacketlen)+4 {
		return true, nil
	}
	//fmt.Printf("rtp channel=%d, rtp size:%d\n", channel, rtppacketlen)
	for i := 0; i < len(c.mediaChanel); i++ {
		if c.mediaChanel[i].RtpChannel == int(channel) {
			if c.mediaChanel[i].rtpdecoder == nil {
				continue
			}
			c.mediaChanel[i].rtpdecoder.decode(c.recvBuf.Bytes()[4 : 4+rtppacketlen])
		}
	}
	c.recvBuf.Next(int(4 + rtppacketlen))
	return false, nil
}

func (c *Rtspclient) handleRtspMessage() (bool, error) {
	var res Response
	state := res.Decode(c.recvBuf.Bytes())
	if state == Failed {
		return false, errors.New("rtsp message error")
	} else if state == InCompleted {
		return true, nil
	}

	if res.StatusCode != "200" && res.StatusCode != "401" {
		return false, errors.New("statuscode is " + res.StatusCode)
	}
	c.recvBuf.Next(res.TotalLen)
	fmt.Println(res.ToString())
	return false, c.handleReponse(res)
}

func (c *Rtspclient) sendRtspCommad(msg []byte) error {
	fmt.Println("send commad " + string(msg))
	var wlen int = 0
	for wlen < len(msg) {
		sendlen, werr := c.conn.Write(msg)
		if werr != nil {
			fmt.Println("write Failed" + werr.Error())
			return errors.New("send rtsp commad faild")
		}
		wlen += sendlen
	}
	return nil
}
