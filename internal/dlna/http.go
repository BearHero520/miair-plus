package dlna

import (
	"context"
	"embed"
	"encoding/xml"
	"fmt"
	"github.com/BearHero520/miair-plus/internal/config"
	"github.com/BearHero520/miair-plus/internal/media"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed scpd/*.xml
var descriptions embed.FS

const protocols = "http-get:*:audio/mpeg:*,http-get:*:audio/mp4:*,http-get:*:audio/aac:*,http-get:*:audio/flac:*,http-get:*:audio/x-flac:*,http-get:*:audio/wav:*,http-get:*:audio/x-wav:*,http-get:*:audio/x-m4a:*,http-get:*:audio/*:*"

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func serviceURN(s string) string { return "urn:schemas-upnp-org:service:" + s + ":1" }
func Description(r *Renderer) string {
	s := r.Snapshot()
	id := s.UDN
	body := `<?xml version="1.0"?><root xmlns="urn:schemas-upnp-org:device-1-0" xmlns:dlna="urn:schemas-dlna-org:device-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><device><deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType><friendlyName>` + esc(s.DLNAName) + `</friendlyName><manufacturer>MiAir Plus</manufacturer><manufacturerURL>https://github.com/BearHero520/miair-plus</manufacturerURL><modelDescription>Xiaomi Audio Bridge</modelDescription><modelName>MiAir Plus</modelName><modelNumber>2.0</modelNumber><UDN>uuid:` + id + `</UDN><dlna:X_DLNADOC>DMR-1.50</dlna:X_DLNADOC><qq:X_QPlay_SoftwareCapability xmlns:qq="http://www.tencent.com">QPlay:2</qq:X_QPlay_SoftwareCapability><serviceList>`
	for _, service := range []string{"AVTransport", "RenderingControl", "ConnectionManager"} {
		prefix := "/dlna/" + id + "/" + service
		body += `<service><serviceType>` + serviceURN(service) + `</serviceType><serviceId>urn:upnp-org:serviceId:` + service + `</serviceId><SCPDURL>` + prefix + `.xml</SCPDURL><controlURL>` + prefix + `/control</controlURL><eventSubURL>` + prefix + `/event</eventSubURL></service>`
	}
	return body + `</serviceList></device></root>`
}
func soapResponse(service, action string, values map[string]string) string {
	body := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` + action + `Response xmlns:u="` + serviceURN(service) + `">`
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		body += "<" + k + ">" + esc(values[k]) + "</" + k + ">"
	}
	return body + "</u:" + action + "Response></s:Body></s:Envelope>"
}
func fault(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(500)
	fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%d</errorCode><errorDescription>%s</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`, code, esc(message))
}

type soapEnvelope struct {
	Body struct {
		Action struct {
			XMLName xml.Name
			Args    []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
			} `xml:",any"`
		} `xml:",any"`
	} `xml:"Body"`
}
type HTTPHandler struct {
	Lookup   func(string) *Renderer
	AutoPlay func() bool
	Events   *Events
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.NotFound(w, req)
		return
	}
	renderer := h.Lookup(parts[1])
	if renderer == nil {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Server", "Linux/3 UPnP/1.0 MiAirPlus/2.0")
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	if req.Method == "GET" && parts[2] == "description.xml" {
		io.WriteString(w, Description(renderer))
		return
	}
	if req.Method == "GET" && strings.HasSuffix(parts[2], ".xml") {
		b, err := descriptions.ReadFile("scpd/" + parts[2])
		if err != nil {
			http.NotFound(w, req)
		} else {
			w.Write(b)
		}
		return
	}
	if len(parts) != 4 {
		http.NotFound(w, req)
		return
	}
	service := parts[2]
	if service != "AVTransport" && service != "RenderingControl" && service != "ConnectionManager" {
		fault(w, 401, "Invalid Service")
		return
	}
	if parts[3] == "event" {
		h.Events.Handle(w, req, renderer, service)
		return
	}
	if req.Method != "POST" || parts[3] != "control" {
		http.Error(w, "method not allowed", 405)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 1<<20))
	if err != nil {
		fault(w, 402, "Invalid Args")
		return
	}
	var envelope soapEnvelope
	if err = xml.Unmarshal(body, &envelope); err != nil {
		fault(w, 402, "Invalid XML")
		return
	}
	action := envelope.Body.Action.XMLName.Local
	header := strings.Trim(strings.TrimSpace(req.Header.Get("SOAPAction")), "\"'")
	urn, headerAction, ok := strings.Cut(header, "#")
	if ok && (urn != serviceURN(service) || headerAction != action) {
		fault(w, 401, "Invalid Action")
		return
	}
	args := map[string]string{}
	for _, a := range envelope.Body.Action.Args {
		args[a.XMLName.Local] = a.Value
	}
	if id, ok := args["InstanceID"]; ok && id != "0" {
		fault(w, 718, "Invalid InstanceID")
		return
	}
	s := renderer.Snapshot()
	result := map[string]string{}
	code := 501
	ctx := req.Context()
	switch service + "/" + action {
	case "AVTransport/SetAVTransportURI":
		err = renderer.SetURI(args["CurrentURI"], args["CurrentURIMetaData"])
		if err == nil && h.AutoPlay != nil && h.AutoPlay() {
			renderer.AutoPlay()
		}
	case "AVTransport/SetNextAVTransportURI":
		err = renderer.SetNext(args["NextURI"], args["NextURIMetaData"])
	case "AVTransport/Play":
		if speed := args["Speed"]; speed != "" && speed != "1" {
			code = 717
			err = fmt.Errorf("unsupported speed")
		} else {
			err = renderer.Play(ctx)
		}
	case "AVTransport/Pause":
		err = renderer.Pause(ctx)
	case "AVTransport/Stop":
		err = renderer.Stop(ctx)
	case "AVTransport/Next":
		err = renderer.Next(ctx)
	case "AVTransport/Previous":
		err = renderer.Seek(ctx, 0)
	case "AVTransport/Seek":
		code = 710
		if args["Unit"] != "REL_TIME" && args["Unit"] != "ABS_TIME" {
			err = fmt.Errorf("unsupported seek mode")
		} else {
			var seconds float64
			seconds, err = media.ParseTime(args["Target"])
			if err == nil {
				err = renderer.Seek(ctx, seconds)
			}
		}
	case "AVTransport/GetTransportInfo":
		result = map[string]string{"CurrentTransportState": s.State, "CurrentTransportStatus": "OK", "CurrentSpeed": "1"}
		if s.Error != "" {
			result["CurrentTransportStatus"] = "ERROR_OCCURRED"
		}
	case "AVTransport/GetPositionInfo":
		result = map[string]string{"Track": "1", "TrackDuration": media.FormatTime(s.Duration), "TrackMetaData": s.Metadata, "TrackURI": s.URI, "RelTime": media.FormatTime(s.Position), "AbsTime": media.FormatTime(s.Position), "RelCount": "2147483647", "AbsCount": "2147483647"}
	case "AVTransport/GetMediaInfo":
		result = map[string]string{"NrTracks": "1", "MediaDuration": media.FormatTime(s.Duration), "CurrentURI": s.URI, "CurrentURIMetaData": s.Metadata, "NextURI": s.NextURI, "NextURIMetaData": "", "PlayMedium": "NETWORK", "RecordMedium": "NOT_IMPLEMENTED", "WriteStatus": "NOT_IMPLEMENTED"}
	case "AVTransport/GetDeviceCapabilities":
		result = map[string]string{"PlayMedia": "NETWORK", "RecMedia": "NOT_IMPLEMENTED", "RecQualityModes": "NOT_IMPLEMENTED"}
	case "AVTransport/GetTransportSettings":
		result = map[string]string{"PlayMode": "NORMAL", "RecQualityMode": "NOT_IMPLEMENTED"}
	case "AVTransport/SetPlayMode":
		if args["NewPlayMode"] != "NORMAL" {
			code = 712
			err = fmt.Errorf("unsupported play mode")
		}
	case "AVTransport/GetCurrentTransportActions":
		result["Actions"] = "Play,Pause,Stop,Seek,Next"
	case "RenderingControl/GetVolume":
		result["CurrentVolume"] = strconv.Itoa(s.Volume)
	case "RenderingControl/SetVolume":
		var volume int
		volume, err = strconv.Atoi(args["DesiredVolume"])
		if err == nil {
			err = renderer.SetVolume(ctx, volume)
		}
		code = 402
	case "RenderingControl/GetMute":
		result["CurrentMute"] = "0"
		if s.Mute {
			result["CurrentMute"] = "1"
		}
	case "RenderingControl/SetMute":
		value := args["DesiredMute"]
		if value != "0" && value != "1" && value != "true" && value != "false" {
			err = fmt.Errorf("invalid mute")
			code = 402
		} else {
			err = renderer.SetMute(ctx, value == "1" || value == "true")
		}
	case "RenderingControl/ListPresets":
		result["CurrentPresetNameList"] = "FactoryDefaults"
	case "ConnectionManager/GetProtocolInfo":
		result = map[string]string{"Source": "", "Sink": protocols}
	case "ConnectionManager/GetCurrentConnectionIDs":
		result["ConnectionIDs"] = "0"
	case "ConnectionManager/GetCurrentConnectionInfo":
		result = map[string]string{"RcsID": "0", "AVTransportID": "0", "ProtocolInfo": "http-get:*:audio/mpeg:*", "PeerConnectionManager": "", "PeerConnectionID": "-1", "Direction": "Input", "Status": "OK"}
	default:
		fault(w, 401, "Invalid Action")
		return
	}
	if err != nil {
		fault(w, code, err.Error())
		return
	}
	io.WriteString(w, soapResponse(service, action, result))
}

type subscription struct {
	sid, url string
	expires  time.Time
	renderer *Renderer
	service  string
	seq      uint32
	queue    chan struct{}
	cancel   context.CancelFunc
}
type Events struct {
	mu     sync.Mutex
	subs   map[string]*subscription
	ctx    context.Context
	client *http.Client
}

func NewEvents(ctx context.Context) *Events {
	return &Events{subs: map[string]*subscription{}, ctx: ctx, client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (e *Events) Changed(r *Renderer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.subs {
		if s.renderer == r {
			select {
			case s.queue <- struct{}{}:
			default:
			}
		}
	}
}
func (e *Events) Handle(w http.ResponseWriter, r *http.Request, renderer *Renderer, service string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	for id, s := range e.subs {
		if s.expires.Before(now) {
			s.cancel()
			delete(e.subs, id)
		}
	}
	sid := r.Header.Get("SID")
	if r.Method == "UNSUBSCRIBE" {
		s, ok := e.subs[sid]
		if !ok || s.renderer != renderer || s.service != service {
			w.WriteHeader(412)
			return
		}
		s.cancel()
		delete(e.subs, sid)
		w.WriteHeader(200)
		return
	}
	if r.Method != "SUBSCRIBE" {
		w.WriteHeader(405)
		return
	}
	seconds := 1800
	if strings.HasPrefix(r.Header.Get("TIMEOUT"), "Second-") {
		v, _ := strconv.Atoi(strings.TrimPrefix(r.Header.Get("TIMEOUT"), "Second-"))
		if v > 0 && v < 1800 {
			seconds = v
		}
	}
	if sid != "" {
		s, ok := e.subs[sid]
		if !ok || s.renderer != renderer || s.service != service {
			w.WriteHeader(412)
			return
		}
		s.expires = now.Add(time.Duration(seconds) * time.Second)
	} else {
		if len(e.subs) >= 128 || r.Header.Get("NT") != "upnp:event" {
			w.WriteHeader(412)
			return
		}
		callback := strings.Trim(r.Header.Get("CALLBACK"), "<>")
		u, err := url.Parse(callback)
		peer, _, _ := net.SplitHostPort(r.RemoteAddr)
		if err != nil || u.Scheme != "http" || u.User != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(peer).Equal(net.ParseIP(u.Hostname())) {
			w.WriteHeader(412)
			return
		}
		sid = "uuid:" + config.Random(16)
		ctx, cancel := context.WithCancel(e.ctx)
		s := &subscription{sid: sid, url: callback, renderer: renderer, service: service, expires: now.Add(time.Duration(seconds) * time.Second), queue: make(chan struct{}, 1), cancel: cancel}
		e.subs[sid] = s
		go e.worker(ctx, s)
		s.queue <- struct{}{}
	}
	w.Header().Set("SID", sid)
	w.Header().Set("TIMEOUT", fmt.Sprintf("Second-%d", seconds))
	w.WriteHeader(200)
}
func (e *Events) worker(ctx context.Context, s *subscription) {
	timer := time.NewTimer(50 * time.Millisecond)
	select {
	case <-ctx.Done():
		timer.Stop()
		return
	case <-timer.C:
	}
	expiry := time.NewTicker(30 * time.Second)
	defer expiry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-expiry.C:
			e.mu.Lock()
			expired := s.expires.Before(time.Now())
			if expired {
				delete(e.subs, s.sid)
			}
			e.mu.Unlock()
			if expired {
				return
			}
		case <-s.queue:
			snapshot := s.renderer.Snapshot()
			properties := ""
			if s.service == "ConnectionManager" {
				properties = "<e:property><SinkProtocolInfo>" + esc(protocols) + "</SinkProtocolInfo></e:property><e:property><SourceProtocolInfo></SourceProtocolInfo></e:property><e:property><CurrentConnectionIDs>0</CurrentConnectionIDs></e:property>"
			} else {
				namespace := "AVT"
				inner := `<TransportState val="` + snapshot.State + `"/><TransportStatus val="OK"/><CurrentTrackURI val="` + esc(snapshot.URI) + `"/><AVTransportURI val="` + esc(snapshot.URI) + `"/><CurrentTrackDuration val="` + media.FormatTime(snapshot.Duration) + `"/><RelativeTimePosition val="` + media.FormatTime(snapshot.Position) + `"/>`
				if s.service == "RenderingControl" {
					namespace = "RCS"
					mute := "0"
					if snapshot.Mute {
						mute = "1"
					}
					inner = fmt.Sprintf(`<Volume channel="Master" val="%d"/><Mute channel="Master" val="%s"/>`, snapshot.Volume, mute)
				}
				last := `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/` + namespace + `/"><InstanceID val="0">` + inner + `</InstanceID></Event>`
				properties = "<e:property><LastChange>" + esc(last) + "</LastChange></e:property>"
			}
			body := `<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">` + properties + `</e:propertyset>`
			req, _ := http.NewRequestWithContext(ctx, "NOTIFY", s.url, strings.NewReader(body))
			req.Header.Set("Content-Type", "text/xml; charset=utf-8")
			req.Header.Set("NT", "upnp:event")
			req.Header.Set("NTS", "upnp:propchange")
			req.Header.Set("SID", s.sid)
			req.Header.Set("SEQ", strconv.FormatUint(uint64(s.seq), 10))
			s.seq++
			if s.seq == 0 {
				s.seq = 1
			}
			resp, err := e.client.Do(req)
			if err == nil {
				io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
				resp.Body.Close()
			}
		}
	}
}
