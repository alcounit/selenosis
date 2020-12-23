package selenium

//Capabilities ...
type Capabilities struct {
	BrowserName           string            `json:"browserName,omitempty"`
	DeviceName            string            `json:"deviceName,omitempty"`
	BrowserVersion        string            `json:"version,omitempty"`
	W3CBrowserVersion     string            `json:"browserVersion,omitempty"`
	Platform              string            `json:"platform,omitempty"`
	WC3PlatformName       string            `json:"platformName,omitempty"`
	ScreenResolution      string            `json:"screenResolution,omitempty"`
	Skin                  string            `json:"skin,omitempty"`
	VNC                   bool              `json:"enableVNC,omitempty"`
	Video                 bool              `json:"enableVideo,omitempty"`
	Log                   bool              `json:"enableLog,omitempty"`
	VideoName             string            `json:"videoName,omitempty"`
	VideoScreenSize       string            `json:"videoScreenSize,omitempty"`
	VideoFrameRate        uint16            `json:"videoFrameRate,omitempty"`
	VideoCodec            string            `json:"videoCodec,omitempty"`
	LogName               string            `json:"logName,omitempty"`
	TestName              string            `json:"name,omitempty"`
	TimeZone              string            `json:"timeZone,omitempty"`
	ContainerHostname     string            `json:"containerHostname,omitempty"`
	Env                   []string          `json:"env,omitempty"`
	ApplicationContainers []string          `json:"applicationContainers,omitempty"`
	AdditionalNetworks    []string          `json:"additionalNetworks,omitempty"`
	HostsEntries          []string          `json:"hostsEntries,omitempty"`
	DNSServers            []string          `json:"dnsServers,omitempty"`
	Labels                map[string]string `json:"labels,omitempty"`
	SessionTimeout        string            `json:"sessionTimeout,omitempty"`
}

//ValidateCapabilities ...
func (c *Capabilities) ValidateCapabilities() {
	if c.BrowserName == "" {
		c.BrowserName = c.DeviceName
	}

	if c.BrowserVersion == "" {
		c.BrowserVersion = c.W3CBrowserVersion
	}

	if c.Platform == "" {
		c.Platform = c.WC3PlatformName
	}

}
