package config

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/mariusor/go-littr/internal/log"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type Configuration struct {
	HostName                   string
	Name                       string
	TimeOut                    time.Duration
	ListenPort                 int
	ListenHost                 string
	APIURL                     string
	Secure                     bool
	CertPath                   string
	KeyPath                    string
	Env                        EnvType
	LogLevel                   log.Level
	AdminContact               string
	AnonymousCommentingEnabled bool
	SessionsEnabled            bool
	VotingEnabled              bool
	DownvotingEnabled          bool
	UserCreatingEnabled        bool
	UserInvitesEnabled         bool
	UserFollowingEnabled       bool
	ModerationEnabled          bool
	MaintenanceMode            bool
}

const (
	DefaultListenPort = 3000
	DefaultListenHost = ""
	Prefix            = "LITTR"
)

const (
	KeyENV                        = "ENV"
	KeyLogLevel                   = "LOG_LEVEL"
	KeyTimeOut                    = "TIME_OUT"
	KeyHostname                   = "HOSTNAME"
	KeyListenHostName             = "LISTEN_HOSTNAME"
	KeyListenPort                 = "LISTEN_PORT"
	KeyName                       = "NAME"
	KeyHTTPS                      = "HTTPS"
	KeyCertPath                   = "CERT_PATH"
	KeyKeyPath                    = "KEY_PATH"
	KeyAPIUrl                     = "API_URL"
	KeyDisableVoting              = "DISABLE_VOTING"
	KeyDisableDownVoting          = "DISABLE_DOWNVOTING"
	KeyDisableSessions            = "DISABLE_SESSIONS"
	KeyDisableUserCreation        = "DISABLE_USER_CREATION"
	KeyDisableUserInvites         = "DISABLE_USER_INVITES"
	KeyDisableAnonymousCommenting = "DISABLE_ANONYMOUS_COMMENTING"
	KeyDisableUserFollowing       = "DISABLE_USER_FOLLOWING"
	KeyDisableModeration          = "DISABLE_MODERATION"
	KeyAdminContact               = "ADMIN_CONTACT"
)

func prefKey(k string) string {
	if Prefix != "" {
		return fmt.Sprintf("%s_%s", strings.ToUpper(Prefix), k)
	}
	return k
}

func loadKeyFromEnv(name, def string) string {
	if val := os.Getenv(prefKey(name)); len(val) > 0 {
		return val
	}
	if val := os.Getenv(name); len(val) > 0 {
		return val
	}
	return def
}

func Load(e EnvType, wait time.Duration) *Configuration {
	c := &Default
	configs := []string{
		".env",
	}
	if !ValidEnv(e) {
		env := loadKeyFromEnv(KeyENV, "")
		e = EnvType(strings.ToLower(env))
	}
	appendIfFile := func(typ EnvType) {
		envFile := fmt.Sprintf(".env.%s", typ)
		if _, err := os.Stat(envFile); err == nil {
			configs = append(configs, envFile)
		}
	}
	if !ValidEnv(e) {
		for _, typ := range validEnvTypes {
			appendIfFile(typ)
		}
	} else {
		appendIfFile(e)
	}
	for _, f := range configs {
		godotenv.Overload(f)
	}
	lvl := loadKeyFromEnv(KeyLogLevel, "INFO")
	switch strings.ToLower(lvl) {
	case "trace":
		c.LogLevel = log.TraceLevel
	case "debug":
		c.LogLevel = log.DebugLevel
	case "warn":
		c.LogLevel = log.WarnLevel
	case "error":
		c.LogLevel = log.ErrorLevel
	case "info":
		fallthrough
	default:
		c.LogLevel = log.InfoLevel
	}
	c.TimeOut = wait
	if to, _ := time.ParseDuration(loadKeyFromEnv(KeyTimeOut, "")); to > 0 {
		c.TimeOut = to
	}
	c.Env = EnvType(os.Getenv("ENV"))
	c.HostName = loadKeyFromEnv(KeyHostname, "")
	c.Name = loadKeyFromEnv(KeyName, c.HostName)
	c.ListenHost = loadKeyFromEnv(KeyListenHostName, DefaultListenHost)
	if port, _ := strconv.ParseInt(loadKeyFromEnv(KeyListenPort, ""), 10, 32); port > 0 {
		c.ListenPort = int(port)
	} else {
		c.ListenPort = DefaultListenPort
	}
	c.KeyPath = path.Clean(loadKeyFromEnv(KeyKeyPath, ""))
	c.CertPath = path.Clean(loadKeyFromEnv(KeyCertPath, ""))

	c.Secure, _ = strconv.ParseBool(loadKeyFromEnv(KeyHTTPS, ""))

	votingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableVoting, ""))
	c.VotingEnabled = !votingDisabled
	if c.VotingEnabled {
		downvotingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableDownVoting, ""))
		c.DownvotingEnabled = !downvotingDisabled
	}
	sessionsDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableSessions, ""))
	c.SessionsEnabled = !sessionsDisabled
	userCreationDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableUserCreation, ""))
	c.UserCreatingEnabled = !userCreationDisabled
	userInvitesDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableUserInvites, ""))
	c.UserInvitesEnabled = !userInvitesDisabled
	// TODO(marius): this stopped working - as the anonymous user doesn't have a valid Outbox.
	anonymousCommentingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableAnonymousCommenting, "")) // DISABLE_ANONYMOUS_COMMENTING
	c.AnonymousCommentingEnabled = !anonymousCommentingDisabled
	userFollowingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableUserFollowing, "")) // DISABLE_USER_FOLLOWING
	c.UserFollowingEnabled = !userFollowingDisabled
	moderationDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableModeration, "")) // DISABLE_MODERATION
	c.ModerationEnabled = !moderationDisabled
	c.AdminContact = loadKeyFromEnv(KeyAdminContact, "") // ADMIN_CONTACT

	c.APIURL = loadKeyFromEnv(KeyAPIUrl, "")

	return c
}

func (c Configuration) Listen() string {
	if len(c.ListenHost) > 0 {
		return fmt.Sprintf("%s:%d", c.ListenHost, c.ListenPort)
	}
	return fmt.Sprintf(":%d", c.ListenPort)
}
