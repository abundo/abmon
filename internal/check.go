package abmon

// Common abmon check functions
// Author: Anders Lowinger, anders@abundo.se

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	dnsnode "github.com/abundo/dnsnode"
	log "github.com/sirupsen/logrus"

	"gopkg.in/yaml.v2"
)

// ===========================================================================
//   Definitions
// ===========================================================================

const (
	DEFAULT_CONFIG_FILE = "/etc/abmon/abmon.yaml"
)

// ----- Configuration file -----
type configTSIG struct {
	Key       string `yaml:"key"`
	Algorithm string `yaml:"algorithm"`
	Secret    string `yaml:"secret"`
}
type ConfigZone struct {
	Name                string
	Customer            string
	Primary             string            `yaml:"primary"`
	DnsNode             bool              `yaml:"dnsnode"`
	DnsCheckPropagation bool              `yaml:"dns_check_propagation"`
	DnsCheckZonemaster  bool              `yaml:"dns_check_zonemaster"`
	Exclude             map[string]string `yaml:"exclude"`
	TSIG                *configTSIG       `yaml:"tsig"`
}

type configIcinga struct {
	URL             string `yaml:"url"`
	IgnoreCertError bool   `yaml:"ignore_cert_error"`
	Username        string `yaml:"username"`
	Password        string `yaml:"password"`
	CommandFile     string `yaml:"command_file"`
}
type configNagios struct {
	CommandFile string `yaml:"command_file"`
}
type configDnsnode struct {
	Token string `yaml:"token"`
	URL   string `yaml:"url"`
}
type configBecs struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}
type ConfigDhcpScope struct {
	Name string
	OID  string `yaml:"oid"`
}
type ConfigFile struct {
	NotifyNagios    bool                        `yaml:"notify_nagios"`
	NotifyIcinga    bool                        `yaml:"notify_icinga"`
	Icinga          configIcinga                `yaml:"icinga"`
	Nagios          configNagios                `yaml:"nagios"`
	Dnsnode         configDnsnode               `yaml:"dnsnode"`
	Becs            configBecs                  `yaml:"becs"`
	Zones           map[string]*ConfigZone      `yaml:"zones"`
	DhcpScopes      map[string]*ConfigDhcpScope `yaml:"dhcp_scopes"`
	Customers       map[string][]*ConfigZone    // calculated
	SortedCustomers []string                    // calculated
}

// nagios/icinga return codes
const (
	OK       int = 0
	WARNING  int = 1
	CRITICAL int = 2
	UNKNOWN  int = 3
)

// Map from Nagios return code as a string, to int
var StrToErrstat = map[string]int{
	"ok":       OK,
	"warning":  WARNING,
	"critical": CRITICAL,
	"unknown":  UNKNOWN,
}

// Map from Nagios return code as a int, to a string
var ErrstatToStr = map[int]string{
	OK:       "OK",
	WARNING:  "WARNING",
	CRITICAL: "CRITICAL",
	UNKNOWN:  "UNKNOWN",
}

// Result sent to nagios/icinga
type MonitoringResult struct {
	Status   int
	Details  []string
	Perfdata []string
}

// Run an external command
// Return output as a string
func RunExternalCommand(cmd string, args []string) (string, error) {
	c := exec.Command(cmd, args...)
	cmdOutput, err := c.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(cmdOutput), nil
}

// Load configuration yaml file
func LoadConfiguration(configFile string) (*ConfigFile, error) {
	var err error
	if configFile == "" {
		configFile = DEFAULT_CONFIG_FILE
	}

	config := new(ConfigFile)
	yamlFile, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlFile, config)
	if err != nil {
		return nil, err
	}

	for name, scope := range config.DhcpScopes {
		scope.Name = name
	}

	// Build map of all customers
	// Key is customer name
	// Value is array of zones
	config.Customers = make(map[string][]*ConfigZone)
	for name, zone := range config.Zones {
		zone.Name = name
		if !strings.HasPrefix(name, "_") {
			_, ok := config.Customers[zone.Customer]
			if !ok {
				config.Customers[zone.Customer] = make([]*ConfigZone, 0)
			}
			config.Customers[zone.Customer] = append(config.Customers[zone.Customer], zone)
		}
	}
	config.SortedCustomers = SortedKeys(config.Customers)
	return config, nil
}

// ===========================================================================
//   Nagios
// ===========================================================================

// Send check result to nagios, using command pipe
func NotifyNagios(config configNagios, status IcingaStatus) {
	panic("Not implemented")
	// f := fmt.Sprintf("[{timestamp}] PROCESS_SERVICE_CHECK_RESULT;{hostname};{service_description};{return_code};{output}\n")
	// s = f.format(**status)
	// open(self.config.command_file, "w").write(s)
}

// Force Nagios to reread the configation files
func ReloadNagios() {
	panic("Not implemented")
}

// ===========================================================================
//   Icinga
// ===========================================================================

type IcingaStatus struct {
	Timestamp          int64
	Hostname           string
	ServiceDescription string
	ReturnCode         int
	PerformanceData    []string
	Output             string
}

// Data structure sent to Icing REST API
type RestIcingaStatus struct {
	Hostname        string   `json:"-"`
	Servicename     string   `json:"-"`
	Type            string   `json:"type"`
	ExitStatus      int      `json:"exit_status"`
	PluginOutput    string   `json:"plugin_output"`
	Filter          string   `json:"filter"`
	PerformanceData []string `json:"performance_data"`
}

// Send status from check to Icinga, using REST API
func NotifyIcinga(config configIcinga, status IcingaStatus) {
	result := RestIcingaStatus{
		Type:            "Service",
		ExitStatus:      status.ReturnCode,
		PluginOutput:    status.Output,
		PerformanceData: status.PerformanceData,
		Filter:          "host.name==\"" + status.Hostname + "\" && service.name==\"" + status.ServiceDescription + "\"",
	}
	// status.Type = "Service"
	// status.Filter = fmt.Sprintf("host.name==\"%s\" && service.name==\"%s\"", status.Hostname, status.Servicename)

	jsonData, err := json.Marshal(result)
	if err != nil {
		log.Error("Error marshaling JSON:", err)
		return
	}

	// Create the API request
	fmt.Println("URL", config.URL)
	fmt.Printf("jsonData %s\n", jsonData)
	req, err := http.NewRequest("POST", config.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error("Error creating request:", err)
		return
	}
	req.SetBasicAuth(config.Username, config.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	// Ignore cert errors?
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Do(req)
	if err != nil {
		log.Error("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Check the response
	if resp.StatusCode == http.StatusOK {
		log.Debug("Check result successfully sent")
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error("Failed to get response body", err)
			return
		}
		// if debug {
		if true {
			log.Debugf("Body: %s", body)
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Failed to send check result, status code: %d\n", resp.StatusCode)
		fmt.Printf("                                        : %s\n", body)
	}
}

// Force Icinga to reread the configation files
//
// A var (rather than a func) so tests can stub it out instead of shelling
// out to systemctl for real.
var ReloadIcinga = func() {
	cmd := exec.Command("systemctl", "reload", "icinga2.service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error reloading icinga:", err, output)
	}
}

// ===========================================================================
//   Monitoring check
// ===========================================================================

// Send status to Nagios/Icinga
func Notify(config *ConfigFile, status IcingaStatus) {
	if config.NotifyIcinga {
		NotifyIcinga(config.Icinga, status)
	}
	if config.NotifyNagios {
		NotifyNagios(config.Nagios, status)
	}
}

// ------ Result -----

func NewResult() *MonitoringResult {
	result := new(MonitoringResult)
	result.Status = UNKNOWN
	return result
}

func (result *MonitoringResult) SetStatus(status int) {
	if status > result.Status {
		result.Status = status
	}
}

func (result *MonitoringResult) AddDetail(msg string) {
	result.Details = append(result.Details, msg)
}

// Print a nagios/icinga check result
// Used when nagios/Icinga forks the check
func (check *MonitoringCheck) Exit(status int, msg string) {
	result := check.Result
	if status >= 0 {
		result.Status = status
	}
	var s strings.Builder
	s.WriteString(ErrstatToStr[result.Status])

	if msg != "" {
		s.WriteString(" ")
		s.WriteString(msg)
	}
	if len(result.Perfdata) > 0 {
		s.WriteString("|")
		s.WriteString(strings.Join(result.Perfdata, " "))
	}
	if len(result.Details) > 0 {
		s.WriteString("\n")
		s.WriteString(strings.Join(result.Details, "\n"))
	}
	fmt.Println(s.String())
	os.Exit(result.Status)
}

// Default CLI options for a check. Embed this anonymously in each check's
// own Opts struct so kong.Parse sees one flat set of flags.
type CheckOpts struct {
	ConfigFile string `help:"Path to configuration file" name:"config" short:"c"`
	Debug      bool   `help:"Enable debug logging" short:"d"`
	Verbose    bool   `help:"Enable verbose output" short:"v"`
	UnknownAs  string `help:"Nagios/Icinga status to report for UNKNOWN" enum:"ok,warning,critical,unknown" default:"unknown"`
	WarningAs  string `help:"Nagios/Icinga status to report for WARNING" enum:"ok,warning,critical,unknown" default:"warning"`
	CriticalAs string `help:"Nagios/Icinga status to report for CRITICAL" enum:"ok,warning,critical,unknown" default:"critical"`
	Loglevel   string `help:"Set log level" short:"l" enum:"error,warning,info,debug" default:"info"`
}

type MonitoringCheck struct {
	Config   *ConfigFile
	Result   *MonitoringResult
	WARNING  int // Return code for WARNING
	CRITICAL int // Return code for CRITICAL
	UNKNOWN  int // Return code for UNKNOWN
	Opts     CheckOpts
}

// Create a Check: initialize log, setup defaults, read configuration file.
// opts.CheckOpts must already be populated by the caller's kong.Parse call.
func NewCheck(opts CheckOpts) (*MonitoringCheck, error) {
	var err error
	check := new(MonitoringCheck)
	check.Opts = opts

	// ----- Logging -----
	myLogFormatter := new(log.TextFormatter)
	myLogFormatter.TimestampFormat = "2006-01-02 15:04:05"
	myLogFormatter.FullTimestamp = true
	log.SetFormatter(myLogFormatter)

	level, ok := dnsnode.Loglevels[check.Opts.Loglevel]
	if !ok {
		log.Fatalf("Error: unknonwn loglevel %s", check.Opts.Loglevel)
	}
	log.SetLevel(level)
	log.Info("Loglevel set to: ", check.Opts.Loglevel)

	// optionally override return codes
	check.WARNING = StrToErrstat[check.Opts.WarningAs]
	check.CRITICAL = StrToErrstat[check.Opts.CriticalAs]
	check.UNKNOWN = StrToErrstat[check.Opts.UnknownAs]

	// Load configuration
	check.Config, err = LoadConfiguration(check.Opts.ConfigFile)
	if err != nil {
		return nil, err
	}

	check.Result = NewResult()

	return check, nil
}

// Nagios range specification
//
//	start < end
//	start and : is not required if start = 0
//	if range is of format "start:" and end is not specified, assume end is infinity
//	todo: to specify negative infinity, use "~"
//	alert is raised if metric is outside start and end range (inclusive of endpoints)
//	todo: if range starts with "@", then alert if inside this range (inclusive of endpoints)
type Range struct {
	Range  string
	Inside bool
	Start  float64
	End    float64
}

func NewRange(range_ string) (*Range, error) {
	r := new(Range)
	r.Range = range_
	if string(range_[0]) == "$" {
		r.Inside = true
		range_ = range_[1:]
	}
	tmp := strings.Split(range_, ":")
	if len(tmp) > 2 {
		return nil, errors.New("range must be one value, or two values separated with colon")
	}
	if len(tmp) == 2 {
		// we have a range
		if tmp[0] != "" {
			r.Start, _ = strconv.ParseFloat(tmp[0], 64)
		} else {
			r.Start = 0
		}
		if tmp[1] != "" {
			r.End, _ = strconv.ParseFloat(tmp[1], 64)
		} else {
			r.End = math.MaxFloat64
		}

	} else {
		// only end is specified
		r.Start = 0
		r.End, _ = strconv.ParseFloat(tmp[0], 64)
	}
	if r.Start > r.End {
		return nil, errors.New("range: start must be lower than end")
	}
	return r, nil
}

// Check if value is outside Start/End
func (r *Range) Check(value float64) bool {
	return value < r.Start || value > r.End
}

// Check if value is critical, warning or ok
func (r *Range) CheckWarnCritical(value float64) int {
	if value > r.End {
		return CRITICAL
	}
	if value > r.Start {
		return WARNING
	}
	return OK
}

/*
class Check_Range:
    """
    Decode a nagios range specification
      start < end
      start and : is not required if start = 0
      if range is of format "start:" and end is not specified, assume end is infinity
      to specify negative infinity, use "~"
      alert is raised if metric is outside start and end range (inclusive of endpoints)
      todo: if range starts with "@", then alert if inside this range (inclusive of endpoints)
    """

    def __init__(self, range_):
        self.range = range_

        tmp = self.range.split(':')
        if len(tmp) > 2:
            raise ValueError('Invalid range')
        if len(tmp) == 2:
            # we have a range
            if tmp[0] != '':
                self.start = float(tmp[0])
            else:
                self.start = 0
            if tmp[1] != '':
                self.end = float(tmp[1])
            else:
                self.end = sys.maxsize
        else:
            # only end is specified
            self.start = 0
            self.end = float(range)

        if self.start > self.end:
            raise ValueError('Start must be lower than end')

    def check(self, value):
        return value < self.start or value > self.end

    def check_warn_crit(self, value):
        '''
        Compare a value, handle start as warning and end as critical
        '''
        if value > self.end:
            return CRITICAL
        if value > self.start:
            return WARNING
        return OK
*/
