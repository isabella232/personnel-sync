package internal

import (
	"encoding/json"
	"log/syslog"

	"github.com/silinternational/personnel-sync/v5/alert"
)

type Person struct {
	CompareValue   string
	ID             string
	Attributes     map[string]string
	DisableChanges bool
}

type AttributeMap struct {
	Source        string
	Destination   string
	Required      bool
	CaseSensitive bool
}

type SourceConfig struct {
	Type      string
	ExtraJSON json.RawMessage
}

type DestinationConfig struct {
	Type          string
	ExtraJSON     json.RawMessage
	DisableAdd    bool
	DisableUpdate bool
	DisableDelete bool
}

const (
	VerbosityLow    = 0
	VerbosityMedium = 5
	VerbosityHigh   = 10
)

type RuntimeConfig struct {
	DryRunMode bool
	Verbosity  int
}

type AppConfig struct {
	Runtime      RuntimeConfig
	Source       SourceConfig
	Destination  DestinationConfig
	Alert        alert.Config
	AttributeMap []AttributeMap
	SyncSets     []SyncSet
}

type SyncSet struct {
	Name        string
	Source      json.RawMessage
	Destination json.RawMessage
}

type ChangeSet struct {
	Create []Person
	Update []Person
	Delete []Person
}

type ChangeResults struct {
	Created uint64
	Updated uint64
	Deleted uint64
}

type EventLogItem struct {
	Message string
	Level   syslog.Priority
}

func (l *EventLogItem) String() string {
	return LogLevels[l.Level] + ": " + l.Message
}

var LogLevels = map[syslog.Priority]string{
	syslog.LOG_EMERG:   "Emerg",
	syslog.LOG_ALERT:   "Alert",
	syslog.LOG_CRIT:    "Critical",
	syslog.LOG_ERR:     "Error",
	syslog.LOG_WARNING: "Warning",
	syslog.LOG_NOTICE:  "Notice",
	syslog.LOG_INFO:    "Info",
	syslog.LOG_DEBUG:   "Debug",
}

type Destination interface {
	ForSet(syncSetJson json.RawMessage) error
	ListUsers(desiredAttrs []string) ([]Person, error)
	ApplyChangeSet(changes ChangeSet, activityLog chan<- EventLogItem) ChangeResults
}

type Source interface {
	ForSet(syncSetJson json.RawMessage) error
	ListUsers(desiredAttrs []string) ([]Person, error)
}
