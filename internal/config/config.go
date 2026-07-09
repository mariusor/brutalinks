package config

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	log "git.sr.ht/~mariusor/lw"
	"github.com/joho/godotenv"
)

type Configuration struct {
	HostName                   string
	BaseURL                    string
	Name                       string
	TimeOut                    time.Duration
	ListenPort                 int
	ListenHost                 string
	APIURL                     string
	Secure                     bool
	CertPath                   string
	KeyPath                    string
	StoragePath                string
	Env                        EnvType
	LogLevel                   log.Level
	AdminContact               string
	AnonymousCommentingEnabled bool
	SessionsEnabled            bool
	VotingEnabled              bool
	DownvotingEnabled          bool
	PublicVotingEnabled        bool
	UserCreatingEnabled        bool
	UserInvitesEnabled         bool
	UserFollowingEnabled       bool
	ModerationEnabled          bool
	CachingEnabled             bool
	AutoAcceptFollows          bool
	MaintenanceMode            bool
	SessionKeys                [][]byte
	SessionsBackend            string
	SessionsPath               string
	Version                    string
}

const (
	DefaultListenPort = 3000
	DefaultListenHost = ""
	Prefix            = "BRUTAL"

	SessionsCookieBackend = "cookie"
	SessionsFSBackend     = "fs"
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
	KeyStoragePath                = "STORAGE_PATH"
	KeyAPIUrl                     = "API_URL"
	KeyDisableVoting              = "DISABLE_VOTING"
	KeyDisableDownVoting          = "DISABLE_DOWNVOTING"
	KeyDisablePublicVoting        = "DISABLE_PUBLIC_VOTING"
	KeyDisableSessions            = "DISABLE_SESSIONS"
	KeyDisableUserCreation        = "DISABLE_USER_CREATION"
	KeyDisableUserInvites         = "DISABLE_USER_INVITES"
	KeyDisableAnonymousCommenting = "DISABLE_ANONYMOUS_COMMENTING"
	KeyDisableUserFollowing       = "DISABLE_USER_FOLLOWING"
	KeyDisableModeration          = "DISABLE_MODERATION"
	KeyDisableCaching             = "DISABLE_CACHING"
	KeyAutoAcceptFollows          = "AUTO_ACCEPT_FOLLOWS"
	KeyAdminContact               = "ADMIN_CONTACT"

	KeyMaintenanceMode = "MAINTENANCE_MODE"

	KeyOAuth2App    = "OAUTH2_APP"
	KeyFedBOXKey    = "OAUTH2_KEY"
	KeyOAuth2Secret = "OAUTH2_SECRET"

	KeySessionAuthKey = "SESS_AUTH_KEY"
	KeySessionEncKey  = "SESS_ENC_KEY"
	KeySessionBackend = "SESSIONS_BACKEND"
	KeySessionPath    = "SESSIONS_PATH"
)

func prefKey(k string) string {
	if Prefix != "" {
		return fmt.Sprintf("%s_%s", strings.ToUpper(Prefix), k)
	}
	return k
}

func loadKeyFromEnv(name, def string) string {
	if val := strings.TrimSpace(os.Getenv(prefKey(name))); len(val) > 0 {
		return val
	}
	if val := strings.TrimSpace(os.Getenv(name)); len(val) > 0 {
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
		godotenv.Load(f)
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
	c.Env = EnvType(strings.ToLower(loadKeyFromEnv(KeyENV, "dev")))
	c.ListenHost = loadKeyFromEnv(KeyListenHostName, DefaultListenHost)
	c.HostName = loadKeyFromEnv(KeyHostname, c.ListenHost)
	c.Name = loadKeyFromEnv(KeyName, c.HostName)
	if port, _ := strconv.ParseInt(loadKeyFromEnv(KeyListenPort, ""), 10, 32); port > 0 {
		c.ListenPort = int(port)
	} else {
		c.ListenPort = DefaultListenPort
	}
	c.KeyPath = path.Clean(loadKeyFromEnv(KeyKeyPath, ""))
	c.CertPath = path.Clean(loadKeyFromEnv(KeyCertPath, ""))
	if sp := path.Clean(loadKeyFromEnv(KeyStoragePath, "")); sp != "." {
		c.StoragePath = sp
	}
	c.Secure, _ = strconv.ParseBool(loadKeyFromEnv(KeyHTTPS, ""))

	votingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableVoting, ""))
	c.VotingEnabled = !votingDisabled
	if c.VotingEnabled {
		publicVotingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisablePublicVoting, ""))
		c.PublicVotingEnabled = !publicVotingDisabled

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
	//anonymousCommentingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableAnonymousCommenting, "true"))
	c.AnonymousCommentingEnabled = false //!anonymousCommentingDisabled
	userFollowingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableUserFollowing, ""))
	c.UserFollowingEnabled = !userFollowingDisabled
	moderationDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableModeration, ""))
	c.ModerationEnabled = !moderationDisabled
	cachingDisabled, _ := strconv.ParseBool(loadKeyFromEnv(KeyDisableCaching, ""))
	c.CachingEnabled = !cachingDisabled

	c.AdminContact = loadKeyFromEnv(KeyAdminContact, "")

	c.APIURL = loadKeyFromEnv(KeyAPIUrl, "")

	c.SessionsBackend = loadKeyFromEnv(KeySessionBackend, SessionsFSBackend)
	c.SessionsPath = loadKeyFromEnv(KeySessionPath, os.TempDir())

	if authKey := loadKeyFromEnv(KeySessionAuthKey, ""); len(authKey) >= 16 {
		c.SessionKeys = append(c.SessionKeys, []byte(authKey[:16]))
	}
	if encKey := loadKeyFromEnv(KeySessionEncKey, ""); len(encKey) >= 16 {
		c.SessionKeys = append(c.SessionKeys, []byte(encKey[:16]))
	}
	c.AutoAcceptFollows, _ = strconv.ParseBool(loadKeyFromEnv(KeyAutoAcceptFollows, ""))
	c.MaintenanceMode, _ = strconv.ParseBool(loadKeyFromEnv(KeyMaintenanceMode, ""))

	return c
}

func (c Configuration) Listen() string {
	if len(c.ListenHost) > 0 {
		return fmt.Sprintf("%s:%d", c.ListenHost, c.ListenPort)
	}
	return fmt.Sprintf(":%d", c.ListenPort)
}
