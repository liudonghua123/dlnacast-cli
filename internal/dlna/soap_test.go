// internal/dlna/soap_test.go
package dlna

import (
	"strings"
	"testing"
)

func TestBuildEnvelope(t *testing.T) {
	env := BuildEnvelope("AVTransport", "Play", map[string]string{"Speed": "1"})
	for _, want := range []string{
		`xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"`,
		"<u:Play>",
		"<Speed>1</Speed>",
		"</u:Play>",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("missing %q in:\n%s", want, env)
		}
	}
}

func TestBuildEnvelopeEscape(t *testing.T) {
	env := BuildEnvelope("AVTransport", "SetAVTransportURI",
		map[string]string{"CurrentURI": "http://x/a.mp4&<b>"})
	if !strings.Contains(env, "a.mp4&amp;&lt;b&gt;") {
		t.Errorf("not escaped:\n%s", env)
	}
}

func TestParseResponse(t *testing.T) {
	body := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"
  s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:GetPositionInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <Track>1</Track>
      <AbsTime>0:00:00</AbsTime>
      <RelTime>0:01:23</RelTime>
    </u:GetPositionInfoResponse>
  </s:Body>
</s:Envelope>`
	args, err := ParseResponse("AVTransport", "GetPositionInfo", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if args["RelTime"] != "0:01:23" {
		t.Errorf("RelTime = %q", args["RelTime"])
	}
	if args["Track"] != "1" {
		t.Errorf("Track = %q", args["Track"])
	}
}

func TestParseFault(t *testing.T) {
	body := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>Invalid Action</faultstring>
      <detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
        <errorCode>401</errorCode><errorDescription>Invalid Action</errorDescription>
      </UPnPError></detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`
	_, err := ParseResponse("AVTransport", "Play", []byte(body))
	if err == nil {
		t.Fatal("want fault error")
	}
	if !strings.Contains(err.Error(), "Invalid Action") {
		t.Errorf("err = %v", err)
	}
}

func TestFormatAndParseTime(t *testing.T) {
	if got := FormatDuration(4529); got != "1:15:29" {
		t.Errorf("FormatDuration = %q", got)
	}
	if got := ParseUPnPTime("1:15:29"); got != 4529 {
		t.Errorf("ParseUPnPTime = %d", got)
	}
	if got := ParseUPnPTime("0:00:00.000"); got != 0 {
		t.Errorf("ParseUPnPTime fractional = %d", got)
	}
}
