package rtsp

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

type ParserState int

const (
	OK ParserState = iota
	InCompleted
	Failed
)

type Message interface {
	Decode(msg []byte) ParserState
	ToString() int
}

type Authenticate interface {
	authenticateInfo() string
	parse(authenticate string) error
}

type DigestAuthenticate struct {
	realm    string
	nonce    string
	username string
	uri      string
	method   string
	password string
}

func (auth DigestAuthenticate) digestInfo() string {
	digest := fmt.Sprintf("Digest username=\"%s\", realm=\"%s\", nonce=\"%s\", uri=\"%s\", response=\"%s\"",
		auth.username, auth.realm, auth.nonce, auth.uri, auth.authenticateInfo())
	return digest
}

//response=md5(md5(username:realm:password):nonce:md5(method:url));
func (auth DigestAuthenticate) authenticateInfo() string {
	str1 := auth.username + ":" + auth.realm + ":" + auth.password
	str2 := auth.method + ":" + auth.uri
	md5Bytes1 := md5.Sum([]byte(str1))
	md5Bytes2 := md5.Sum([]byte(str2))
	md5str1 := fmt.Sprintf("%x", md5Bytes1)
	md5str2 := fmt.Sprintf("%x", md5Bytes2)
	str3 := md5str1 + ":" + auth.nonce + ":" + md5str2
	md5Bytes3 := md5.Sum([]byte(str3))
	response := fmt.Sprintf("%x", md5Bytes3)
	return response
}

func (auth *DigestAuthenticate) parse(authenticate string) error {
	elems := strings.Split(authenticate, ",")
	for i := 0; i < len(elems); i++ {
		elem := strings.TrimSpace(elems[i])
		if strings.Contains(elem, "realm") {
			fmt.Sscanf(elem, "realm=\"%s\"", &auth.realm)
			auth.realm = auth.realm[:len(auth.realm)-1]
		} else if strings.Contains(elem, "nonce") {
			fmt.Sscanf(elem, "nonce=\"%s\"", &auth.nonce)
			auth.nonce = auth.nonce[:len(auth.nonce)-1]
		}
	}

	return nil
}

type Request struct {
	Method       string
	Uri          string
	Version      string
	HeaderFileds map[string]string
	Body         []byte
}

type Response struct {
	Version      string
	StatusCode   string
	Reason       string
	HeaderFileds map[string]string
	Body         []byte
	TotalLen     int
}

type Transport interface {
	Parser(transport string) int
	ToString() string
}

type TcpTransport struct {
	Interleaved [2]int
	SSRC        string
	Mode        string
}

func (t *TcpTransport) Parser(transport string) int {
	params := strings.Split(transport, ";")
	for idx := range params {
		param := params[idx]
		if param == "RTP/AVP" {
			return -1
		} else if strings.Contains(param, "interleaved") {
			fmt.Sscanf(param, "interleaved=%d-%d", &t.Interleaved[0], &t.Interleaved[1])
		} else if strings.Contains(param, "ssrc") {
			fmt.Sscanf(param, "ssrc=%s", &t.SSRC)
		} else if strings.Contains(param, "mode") {
			fmt.Sscanf(param, "mode=%s", &t.Mode)
		}
	}
	return 0
}

func (t TcpTransport) ToString() string {
	var transport string
	transport = "RTP/AVP/TCP;unicast"
	transport += ";interleaved=" + strconv.Itoa(t.Interleaved[0]) + "-" + strconv.Itoa(t.Interleaved[1])
	if t.SSRC != "" {
		transport += ";ssrc=" + t.SSRC
	}

	if t.Mode != "" {
		transport += ";mode=" + t.Mode
	}

	return transport
}

func (req Request) Decode(msg []byte) ParserState {
	return OK
}

func (req *Request) ToString() string {
	var request string
	request = req.Method + " " + req.Uri + " " + req.Version + "\r\n"
	for k, v := range req.HeaderFileds {
		request += k + ": " + v + "\r\n"
	}
	request += "\r\n"
	request += string(req.Body)
	return request
}

func MakeOption(uri string) Request {
	var req Request
	req.Method = "OPTIONS"
	req.Uri = uri
	req.Version = "RTSP/1.0"
	req.HeaderFileds = make(map[string]string)
	req.HeaderFileds["Content-Length"] = "0"
	req.HeaderFileds["Date"] = time.Now().UTC().Format("02 Jan 06 15:04:05 GMT")
	return req
}

func MakeDescribe(uri string) Request {
	var req Request
	req.Method = "DESCRIBE"
	req.Uri = uri
	req.Version = "RTSP/1.0"
	req.HeaderFileds = make(map[string]string)
	req.HeaderFileds["Content-Length"] = "0"
	req.HeaderFileds["Accept"] = "application/sdp"
	req.HeaderFileds["Date"] = time.Now().UTC().Format("02 Jan 06 15:04:05 GMT")
	return req
}

func MakeSetup(uri string) Request {
	var req Request
	req.Method = "SETUP"
	req.Uri = uri
	req.Version = "RTSP/1.0"
	req.HeaderFileds = make(map[string]string)
	req.HeaderFileds["Content-Length"] = "0"
	req.HeaderFileds["Date"] = time.Now().UTC().Format("02 Jan 06 15:04:05 GMT")
	return req
}

func MakePlay(uri string) Request {
	var req Request
	req.Method = "PLAY"
	req.Uri = uri
	req.Version = "RTSP/1.0"
	req.HeaderFileds = make(map[string]string)
	req.HeaderFileds["Content-Length"] = "0"
	req.HeaderFileds["Accept"] = "application/sdp"
	req.HeaderFileds["Date"] = time.Now().UTC().Format("02 Jan 06 15:04:05 GMT")
	return req
}

func MakeTearDown(uri string) Request {
	var req Request
	req.Method = "TEARDOWN"
	req.Uri = uri
	req.Version = "RTSP/1.0"
	req.HeaderFileds = make(map[string]string)
	req.HeaderFileds["Content-Length"] = "0"
	req.HeaderFileds["Date"] = time.Now().UTC().Format("02 Jan 06 15:04:05 GMT")
	return req
}

func (res *Response) Decode(msg []byte) ParserState {
	ret := bytes.HasPrefix(msg, []byte("RTSP/1.0"))
	if !ret {
		log.Println("message have no RTSP/1.0")
		return Failed
	}
	idx := bytes.Index(msg, []byte("\r\n\r\n"))
	if idx == -1 {
		if len(msg) > 8196 {
			log.Println("message too large")
			return Failed
		} else {
			return InCompleted
		}
	}
	reslines := bytes.Split(msg[:idx], []byte("\r\n"))
	if len(reslines) < 1 {
		log.Println("res line is to small ")
		return Failed
	}
	firstline := reslines[0]
	elems := bytes.Split(firstline, []byte(" "))
	if len(elems) < 3 {
		log.Println(("elem too small"))
		return Failed
	}
	res.Version = string(elems[0])
	res.StatusCode = string(elems[1])
	res.Reason = string(elems[2])
	res.HeaderFileds = make(map[string]string)
	for i := 1; i < len(reslines); i++ {
		kv := bytes.SplitN(reslines[i], []byte(":"), 2)
		if len(kv) < 2 {
			log.Println("have no kv")
			return Failed
		}
		key := kv[0]
		value := bytes.TrimSpace(kv[1])
		res.HeaderFileds[string(key)] = string(value)
	}
	res.TotalLen = idx + 4
	length, ok := res.HeaderFileds["Content-Length"]
	if ok {
		contentlen, err := strconv.Atoi(length)
		if err != nil {
			log.Println("no content length")
			return Failed
		}
		res.Body = msg[idx+4 : idx+4+contentlen]
		res.TotalLen += contentlen
	}

	return OK
}

func (res *Response) ToString() string {
	var response string
	response = res.Version + " " + res.StatusCode + " " + res.Reason + "\r\n"
	for k, v := range res.HeaderFileds {
		response += k + ": " + v + "\r\n"
	}
	response += "\r\n"
	response += string(res.Body)
	return response
}
