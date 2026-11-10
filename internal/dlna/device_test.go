// internal/dlna/device_test.go
package dlna

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const descXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <friendlyName>客厅电视</friendlyName>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
        <controlURL>/ctl/RC</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <controlURL>/ctl/AVT</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
        <controlURL>/ctl/CM</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

func TestDescribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(descXML))
	}))
	defer srv.Close()

	dev, err := Describe(srv.URL, NewClient())
	if err != nil {
		t.Fatal(err)
	}
	if dev.Name != "客厅电视" {
		t.Errorf("Name = %q", dev.Name)
	}
	if !strings.HasSuffix(dev.ControlURL, "/ctl/AVT") || !strings.HasPrefix(dev.ControlURL, "http://") {
		t.Errorf("ControlURL = %q", dev.ControlURL)
	}
	if !strings.HasSuffix(dev.RenderingControlURL, "/ctl/RC") {
		t.Errorf("RenderingControlURL = %q", dev.RenderingControlURL)
	}
}

func TestDescribeNoAVTransport(t *testing.T) {
	badXML := `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device><friendlyName>X</friendlyName></device>
</root>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(badXML))
	}))
	defer srv.Close()
	_, err := Describe(srv.URL, NewClient())
	if err == nil || !strings.Contains(err.Error(), "AVTransport") {
		t.Errorf("err = %v", err)
	}
}
