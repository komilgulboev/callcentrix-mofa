package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr       string
	PublicBase     string
	DBDSN          string
	JWTSecret      string
	JWTMinutes     int
	AMIAddr        string
	AMIUser        string
	AMIPass        string
	MinioEndpoint  string // host:port, no scheme — e.g. "minio.internal:9000"
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool
	AsteriskWSURI  string // wss://host:8089/ws
	SIPDomain      string
	UploadsDir     string
	SIPTransport   string
	AsteriskKey    string
	CDRDSN         string
	StaticDir      string
	TLSCertFile    string // if set (with TLSKeyFile), serve HTTPS instead of plain HTTP
	TLSKeyFile     string
}

func Load() *Config {
	// Загружаем .env если есть (в продакшене можно пропустить)
	_ = godotenv.Load()

	ttl, _ := strconv.Atoi(getEnv("JWT_TTL_MINUTES", "60"))
	useSSL, _ := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	return &Config{
		HTTPAddr:       getEnv("HTTP_ADDR", ":8080"),
		PublicBase:     getEnv("HTTP_PUBLIC_BASE", "http://localhost:8080"),
		DBDSN:          getEnv("DB_DSN", ""),
		JWTSecret:      getEnv("JWT_SECRET", "change_me"),
		JWTMinutes:     ttl,
		AMIAddr:        getEnv("AMI_ADDR", "127.0.0.1:5038"),
		AMIUser:        getEnv("AMI_USER", "asterisk"),
		AMIPass:        getEnv("AMI_PASS", "asterisk"),
		MinioEndpoint:  getEnv("MINIO_ENDPOINT", ""),
		MinioAccessKey: getEnv("MINIO_ACCESS_KEY", ""),
		MinioSecretKey: getEnv("MINIO_SECRET_KEY", ""),
		MinioBucket:    getEnv("MINIO_BUCKET", "recordings"),
		MinioUseSSL:    useSSL,
		AsteriskWSURI:  getEnv("ASTERISK_WS_URI", "wss://localhost:8089/ws"),
		SIPDomain:      getEnv("SIP_DOMAIN", "localhost"),
		UploadsDir:     getEnv("UPLOADS_DIR", "./uploads"),
		SIPTransport:   getEnv("SIP_TRANSPORT", "transport-wss"),
		AsteriskKey:    getEnv("ASTERISK_KEY", ""),
		CDRDSN:         getEnv("CDR_DB_DSN", ""),
		StaticDir:      getEnv("STATIC_DIR", "./web"),
		TLSCertFile:    getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:     getEnv("TLS_KEY_FILE", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
