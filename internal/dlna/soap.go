// internal/dlna/soap.go
package dlna

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct{ HTTP *http.Client }

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 5 * time.Second}}
}

func BuildEnvelope(service, action string, args map[string]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/" xmlns:u="urn:schemas-upnp-org:service:` + service + `:1"><s:Body><u:`)
	b.WriteString(action)
	b.WriteString(">")
	for k, v := range args {
		b.WriteString("<" + k + ">" + xmlEscape(v) + "</" + k + ">")
	}
	b.WriteString("</u:" + action + "></s:Body></s:Envelope>")
	return b.String()
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func ParseResponse(service, action string, body []byte) (map[string]string, error) {
	var fault struct {
		Body struct {
			Fault struct {
				FaultString string `xml:"faultstring"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(body, &fault); err == nil && fault.Body.Fault.FaultString != "" {
		return nil, fmt.Errorf("SOAP fault: %s", fault.Body.Fault.FaultString)
	}

	wrapper := struct {
		Body struct {
			InnerXML []byte `xml:",innerxml"`
		} `xml:"Body"`
	}{}
	if err := xml.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	result := map[string]string{}
	decoder := xml.NewDecoder(bytes.NewReader(wrapper.Body.InnerXML))
	// depth 1 = "*Response" wrapper, depth 2 = its direct argument elements.
	depth := 0
	var curName string
	var curText strings.Builder
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				curName = t.Name.Local
				curText.Reset()
			}
		case xml.EndElement:
			if depth == 2 {
				result[curName] = curText.String()
			}
			depth--
		case xml.CharData:
			if depth >= 2 {
				curText.Write(t)
			}
		}
	}
	return result, nil
}

func (c *Client) Call(controlURL, service, action string, args map[string]string) (map[string]string, error) {
	env := BuildEnvelope(service, action, args)
	req, err := http.NewRequest("POST", controlURL, strings.NewReader(env))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:`+service+`:1#`+action+`"`)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		if _, perr := ParseResponse(service, action, body); perr != nil {
			return nil, fmt.Errorf("%s: HTTP %d: %v", action, resp.StatusCode, perr)
		}
		return nil, fmt.Errorf("%s: HTTP %d", action, resp.StatusCode)
	}
	return ParseResponse(service, action, body)
}

// FormatDuration 把秒格式化为 UPnP H:MM:SS。
func FormatDuration(sec int) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}

// ParseUPnPTime 把 "H:MM:SS"（可带 .mmm 小数）解析为秒。
func ParseUPnPTime(s string) int {
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	sec, _ := strconv.Atoi(parts[2])
	return h*3600 + m*60 + sec
}
