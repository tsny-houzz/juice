package main

// -------- Types to match the provided JSON --------

type LogRecord struct {
	Application string         `json:"application"`
	Component   string         `json:"component"`
	Environment EnvInfo        `json:"environment"`
	Level       string         `json:"level"`
	LevelMeta   LevelMeta      `json:"levelMeta"`
	LogVersion  string         `json:"logVersion"`
	Message     string         `json:"message"`
	Metadata    Metadata       `json:"metadata"`
	SpanID      string         `json:"spanId"`
	Stack       string         `json:"stack"`
	Timestamp   int64          `json:"timestamp"`
	TraceID     string         `json:"traceId"`
	Extras      map[string]any `json:"-"` // catch-alls if needed
}

type LevelMeta struct {
	Request LevelMetaRequest `json:"request"`
}

type LevelMetaRequest struct {
	CommandName string `json:"commandName"`
	Host        string `json:"host"`
	URL         string `json:"url"`
}

type EnvInfo struct {
	Arch    string `json:"arch"`
	Cluster string `json:"cluster"`
	Pool    string `json:"pool"`
	Server  string `json:"server"`
	Version string `json:"version"`
}

type Geo struct {
	Addr         string `json:"addr"`
	City         string `json:"city"`
	Country      string `json:"country"`
	DMA          string `json:"dma"`
	LocationInfo any    `json:"locationInfo"`
	PostalCode   string `json:"postalCode"`
	Region       string `json:"region"`
	TimeZone     string `json:"timeZone"`
}

type Metadata struct {
	ClientIP              string `json:"client-ip"`
	CommandName           string `json:"command-name"`
	Date                  string `json:"date"`
	Domain                string `json:"domain"`
	Geo                   Geo    `json:"geo"`
	HTTPVersion           string `json:"http-version"`
	Method                string `json:"method"`
	Referrer              string `json:"referrer"`
	RemoteAddr            string `json:"remote-addr"`
	RequestID             string `json:"request-id"`
	ResponseContentLength string `json:"response-content-length"`
	ResponseTimeMS        string `json:"response-timeMS"`
	Status                string `json:"status"`
	URL                   string `json:"url"`
	UserAgent             string `json:"user-agent"`
}
