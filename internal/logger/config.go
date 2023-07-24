// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package logger

import (
	"crypto/tls"
	"errors"
	"strconv"
	"strings"

	"github.com/minio/pkg/env"
	xnet "github.com/minio/pkg/net"

	"github.com/minio/minio/internal/config"
	"github.com/minio/minio/internal/logger/target/http"
	"github.com/minio/minio/internal/logger/target/kafka"
)

// Console logger target
type Console struct {
	Enabled bool `json:"enabled"`
}

// Audit/Logger constants
const (
	Endpoint   = "endpoint"
	AuthToken  = "auth_token"
	ClientCert = "client_cert"
	ClientKey  = "client_key"
	QueueSize  = "queue_size"
	QueueDir   = "queue_dir"
	Proxy      = "proxy"

	KafkaBrokers       = "brokers"
	KafkaTopic         = "topic"
	KafkaTLS           = "tls"
	KafkaTLSSkipVerify = "tls_skip_verify"
	KafkaTLSClientAuth = "tls_client_auth"
	KafkaSASL          = "sasl"
	KafkaSASLUsername  = "sasl_username"
	KafkaSASLPassword  = "sasl_password"
	KafkaSASLMechanism = "sasl_mechanism"
	KafkaClientTLSCert = "client_tls_cert"
	KafkaClientTLSKey  = "client_tls_key"
	KafkaVersion       = "version"
	KafkaQueueDir      = "queue_dir"
	KafkaQueueSize     = "queue_size"

	EnvLoggerWebhookEnable     = "MINIO_LOGGER_WEBHOOK_ENABLE"
	EnvLoggerWebhookEndpoint   = "MINIO_LOGGER_WEBHOOK_ENDPOINT"
	EnvLoggerWebhookAuthToken  = "MINIO_LOGGER_WEBHOOK_AUTH_TOKEN"
	EnvLoggerWebhookClientCert = "MINIO_LOGGER_WEBHOOK_CLIENT_CERT"
	EnvLoggerWebhookClientKey  = "MINIO_LOGGER_WEBHOOK_CLIENT_KEY"
	EnvLoggerWebhookProxy      = "MINIO_LOGGER_WEBHOOK_PROXY"
	EnvLoggerWebhookQueueSize  = "MINIO_LOGGER_WEBHOOK_QUEUE_SIZE"
	EnvLoggerWebhookQueueDir   = "MINIO_LOGGER_WEBHOOK_QUEUE_DIR"

	EnvAuditWebhookEnable     = "MINIO_AUDIT_WEBHOOK_ENABLE"
	EnvAuditWebhookEndpoint   = "MINIO_AUDIT_WEBHOOK_ENDPOINT"
	EnvAuditWebhookAuthToken  = "MINIO_AUDIT_WEBHOOK_AUTH_TOKEN"
	EnvAuditWebhookClientCert = "MINIO_AUDIT_WEBHOOK_CLIENT_CERT"
	EnvAuditWebhookClientKey  = "MINIO_AUDIT_WEBHOOK_CLIENT_KEY"
	EnvAuditWebhookQueueSize  = "MINIO_AUDIT_WEBHOOK_QUEUE_SIZE"
	EnvAuditWebhookQueueDir   = "MINIO_AUDIT_WEBHOOK_QUEUE_DIR"

	EnvKafkaEnable        = "MINIO_AUDIT_KAFKA_ENABLE"
	EnvKafkaBrokers       = "MINIO_AUDIT_KAFKA_BROKERS"
	EnvKafkaTopic         = "MINIO_AUDIT_KAFKA_TOPIC"
	EnvKafkaTLS           = "MINIO_AUDIT_KAFKA_TLS"
	EnvKafkaTLSSkipVerify = "MINIO_AUDIT_KAFKA_TLS_SKIP_VERIFY"
	EnvKafkaTLSClientAuth = "MINIO_AUDIT_KAFKA_TLS_CLIENT_AUTH"
	EnvKafkaSASLEnable    = "MINIO_AUDIT_KAFKA_SASL"
	EnvKafkaSASLUsername  = "MINIO_AUDIT_KAFKA_SASL_USERNAME"
	EnvKafkaSASLPassword  = "MINIO_AUDIT_KAFKA_SASL_PASSWORD"
	EnvKafkaSASLMechanism = "MINIO_AUDIT_KAFKA_SASL_MECHANISM"
	EnvKafkaClientTLSCert = "MINIO_AUDIT_KAFKA_CLIENT_TLS_CERT"
	EnvKafkaClientTLSKey  = "MINIO_AUDIT_KAFKA_CLIENT_TLS_KEY"
	EnvKafkaVersion       = "MINIO_AUDIT_KAFKA_VERSION"
	EnvKafkaQueueDir      = "MINIO_AUDIT_KAFKA_QUEUE_DIR"
	EnvKafkaQueueSize     = "MINIO_AUDIT_KAFKA_QUEUE_SIZE"

	loggerTargetNamePrefix = "logger-"
	auditTargetNamePrefix  = "audit-"
)

// Default KVS for loggerHTTP and loggerAuditHTTP
var (
	DefaultLoggerWebhookKVS = config.KVS{
		config.KV{
			Key:   config.Enable,
			Value: config.EnableOff,
		},
		config.KV{
			Key:   Endpoint,
			Value: "",
		},
		config.KV{
			Key:   AuthToken,
			Value: "",
		},
		config.KV{
			Key:   ClientCert,
			Value: "",
		},
		config.KV{
			Key:   ClientKey,
			Value: "",
		},
		config.KV{
			Key:   Proxy,
			Value: "",
		},
		config.KV{
			Key:   QueueSize,
			Value: "100000",
		},
		config.KV{
			Key:   QueueDir,
			Value: "",
		},
	}

	DefaultAuditWebhookKVS = config.KVS{
		config.KV{
			Key:   config.Enable,
			Value: config.EnableOff,
		},
		config.KV{
			Key:   Endpoint,
			Value: "",
		},
		config.KV{
			Key:   AuthToken,
			Value: "",
		},
		config.KV{
			Key:   ClientCert,
			Value: "",
		},
		config.KV{
			Key:   ClientKey,
			Value: "",
		},
		config.KV{
			Key:   QueueSize,
			Value: "100000",
		},
		config.KV{
			Key:   QueueDir,
			Value: "",
		},
	}

	DefaultAuditKafkaKVS = config.KVS{
		config.KV{
			Key:   config.Enable,
			Value: config.EnableOff,
		},
		config.KV{
			Key:   KafkaTopic,
			Value: "",
		},
		config.KV{
			Key:   KafkaBrokers,
			Value: "",
		},
		config.KV{
			Key:   KafkaSASLUsername,
			Value: "",
		},
		config.KV{
			Key:   KafkaSASLPassword,
			Value: "",
		},
		config.KV{
			Key:   KafkaSASLMechanism,
			Value: "plain",
		},
		config.KV{
			Key:   KafkaClientTLSCert,
			Value: "",
		},
		config.KV{
			Key:   KafkaClientTLSKey,
			Value: "",
		},
		config.KV{
			Key:   KafkaTLSClientAuth,
			Value: "0",
		},
		config.KV{
			Key:   KafkaSASL,
			Value: config.EnableOff,
		},
		config.KV{
			Key:   KafkaTLS,
			Value: config.EnableOff,
		},
		config.KV{
			Key:   KafkaTLSSkipVerify,
			Value: config.EnableOff,
		},
		config.KV{
			Key:   KafkaVersion,
			Value: "",
		},
		config.KV{
			Key:   QueueSize,
			Value: "100000",
		},
		config.KV{
			Key:   QueueDir,
			Value: "",
		},
	}
)

// Config console and http logger targets
type Config struct {
	Console      Console                 `json:"console"`
	HTTP         map[string]http.Config  `json:"http"`
	AuditWebhook map[string]http.Config  `json:"audit"`
	AuditKafka   map[string]kafka.Config `json:"audit_kafka"`
}

// NewConfig - initialize new logger config.
func NewConfig() Config {
	cfg := Config{
		// Console logging is on by default
		Console: Console{
			Enabled: true,
		},
		HTTP:         make(map[string]http.Config),
		AuditWebhook: make(map[string]http.Config),
		AuditKafka:   make(map[string]kafka.Config),
	}

	return cfg
}

func getCfgVal(envName, key, defaultValue string) string {
	if key != config.Default {
		envName = envName + config.Default + key
	}
	return env.Get(envName, defaultValue)
}

func lookupLegacyConfigForSubSys(subSys string) Config {
	cfg := NewConfig()
	switch subSys {
	case config.LoggerWebhookSubSys:
		var loggerTargets []string
		envs := env.List(legacyEnvLoggerHTTPEndpoint)
		for _, k := range envs {
			target := strings.TrimPrefix(k, legacyEnvLoggerHTTPEndpoint+config.Default)
			if target == legacyEnvLoggerHTTPEndpoint {
				target = config.Default
			}
			loggerTargets = append(loggerTargets, target)
		}

		// Load HTTP logger from the environment if found
		for _, target := range loggerTargets {
			endpoint := getCfgVal(legacyEnvLoggerHTTPEndpoint, target, "")
			if endpoint == "" {
				continue
			}
			cfg.HTTP[target] = http.Config{
				Enabled:  true,
				Endpoint: endpoint,
			}
		}

	case config.AuditWebhookSubSys:
		// List legacy audit ENVs if any.
		var loggerAuditTargets []string
		envs := env.List(legacyEnvAuditLoggerHTTPEndpoint)
		for _, k := range envs {
			target := strings.TrimPrefix(k, legacyEnvAuditLoggerHTTPEndpoint+config.Default)
			if target == legacyEnvAuditLoggerHTTPEndpoint {
				target = config.Default
			}
			loggerAuditTargets = append(loggerAuditTargets, target)
		}

		for _, target := range loggerAuditTargets {
			endpoint := getCfgVal(legacyEnvAuditLoggerHTTPEndpoint, target, "")
			if endpoint == "" {
				continue
			}
			cfg.AuditWebhook[target] = http.Config{
				Enabled:  true,
				Endpoint: endpoint,
			}
		}

	}
	return cfg
}

func lookupAuditKafkaConfig(scfg config.Config, cfg Config) (Config, error) {
	for k, kv := range config.Merge(scfg[config.AuditKafkaSubSys], EnvKafkaEnable, DefaultAuditKafkaKVS) {
		enabledCfgVal := getCfgVal(EnvKafkaEnable, k, kv.Get(config.Enable))
		enabled, err := config.ParseBool(enabledCfgVal)
		if err != nil {
			return cfg, err
		}
		if !enabled {
			continue
		}
		var brokers []xnet.Host
		kafkaBrokers := getCfgVal(EnvKafkaBrokers, k, kv.Get(KafkaBrokers))
		if len(kafkaBrokers) == 0 {
			return cfg, config.Errorf("kafka 'brokers' cannot be empty")
		}
		for _, s := range strings.Split(kafkaBrokers, config.ValueSeparator) {
			var host *xnet.Host
			host, err = xnet.ParseHost(s)
			if err != nil {
				break
			}
			brokers = append(brokers, *host)
		}
		if err != nil {
			return cfg, err
		}

		clientAuthCfgVal := getCfgVal(EnvKafkaTLSClientAuth, k, kv.Get(KafkaTLSClientAuth))
		clientAuth, err := strconv.Atoi(clientAuthCfgVal)
		if err != nil {
			return cfg, err
		}

		kafkaArgs := kafka.Config{
			Enabled: enabled,
			Brokers: brokers,
			Topic:   getCfgVal(EnvKafkaTopic, k, kv.Get(KafkaTopic)),
			Version: getCfgVal(EnvKafkaVersion, k, kv.Get(KafkaVersion)),
		}

		kafkaArgs.TLS.Enable = getCfgVal(EnvKafkaTLS, k, kv.Get(KafkaTLS)) == config.EnableOn
		kafkaArgs.TLS.SkipVerify = getCfgVal(EnvKafkaTLSSkipVerify, k, kv.Get(KafkaTLSSkipVerify)) == config.EnableOn
		kafkaArgs.TLS.ClientAuth = tls.ClientAuthType(clientAuth)

		kafkaArgs.TLS.ClientTLSCert = getCfgVal(EnvKafkaClientTLSCert, k, kv.Get(KafkaClientTLSCert))
		kafkaArgs.TLS.ClientTLSKey = getCfgVal(EnvKafkaClientTLSKey, k, kv.Get(KafkaClientTLSKey))

		kafkaArgs.SASL.Enable = getCfgVal(EnvKafkaSASLEnable, k, kv.Get(KafkaSASL)) == config.EnableOn
		kafkaArgs.SASL.User = getCfgVal(EnvKafkaSASLUsername, k, kv.Get(KafkaSASLUsername))
		kafkaArgs.SASL.Password = getCfgVal(EnvKafkaSASLPassword, k, kv.Get(KafkaSASLPassword))
		kafkaArgs.SASL.Mechanism = getCfgVal(EnvKafkaSASLMechanism, k, kv.Get(KafkaSASLMechanism))

		kafkaArgs.QueueDir = getCfgVal(EnvKafkaQueueDir, k, kv.Get(KafkaQueueDir))

		queueSizeCfgVal := getCfgVal(EnvKafkaQueueSize, k, kv.Get(KafkaQueueSize))
		queueSize, err := strconv.Atoi(queueSizeCfgVal)
		if err != nil {
			return cfg, err
		}
		if queueSize <= 0 {
			return cfg, errors.New("invalid queue_size value")
		}
		kafkaArgs.QueueSize = queueSize

		cfg.AuditKafka[k] = kafkaArgs
	}

	return cfg, nil
}

func lookupLoggerWebhookConfig(scfg config.Config, cfg Config) (Config, error) {
	envs := env.List(EnvLoggerWebhookEndpoint)
	var loggerTargets []string
	for _, k := range envs {
		target := strings.TrimPrefix(k, EnvLoggerWebhookEndpoint+config.Default)
		if target == EnvLoggerWebhookEndpoint {
			target = config.Default
		}
		loggerTargets = append(loggerTargets, target)
	}

	// Load HTTP logger from the environment if found
	for _, target := range loggerTargets {
		if v, ok := cfg.HTTP[target]; ok && v.Enabled {
			// This target is already enabled using the
			// legacy environment variables, ignore.
			continue
		}

		enableCfgVal := getCfgVal(EnvLoggerWebhookEnable, target, "")
		enable, err := config.ParseBool(enableCfgVal)
		if err != nil || !enable {
			continue
		}

		clientCert := getCfgVal(EnvLoggerWebhookClientCert, target, "")
		clientKey := getCfgVal(EnvLoggerWebhookClientKey, target, "")
		err = config.EnsureCertAndKey(clientCert, clientKey)
		if err != nil {
			return cfg, err
		}

		queueSizeCfgVal := getCfgVal(EnvLoggerWebhookQueueSize, target, "100000")
		queueSize, err := strconv.Atoi(queueSizeCfgVal)
		if err != nil {
			return cfg, err
		}
		if queueSize <= 0 {
			return cfg, errors.New("invalid queue_size value")
		}

		cfg.HTTP[target] = http.Config{
			Enabled:    true,
			Endpoint:   getCfgVal(EnvLoggerWebhookEndpoint, target, ""),
			AuthToken:  getCfgVal(EnvLoggerWebhookAuthToken, target, ""),
			ClientCert: clientCert,
			ClientKey:  clientKey,
			Proxy:      getCfgVal(EnvLoggerWebhookProxy, target, ""),
			QueueSize:  queueSize,
			QueueDir:   getCfgVal(EnvLoggerWebhookQueueDir, target, ""),
			Name:       loggerTargetNamePrefix + target,
		}
	}

	for starget, kv := range scfg[config.LoggerWebhookSubSys] {
		if l, ok := cfg.HTTP[starget]; ok && l.Enabled {
			// Ignore this HTTP logger config since there is
			// a target with the same name loaded and enabled
			// from the environment.
			continue
		}
		subSysTarget := config.LoggerWebhookSubSys
		if starget != config.Default {
			subSysTarget = config.LoggerWebhookSubSys + config.SubSystemSeparator + starget
		}
		if err := config.CheckValidKeys(subSysTarget, kv, DefaultLoggerWebhookKVS); err != nil {
			return cfg, err
		}
		enabled, err := config.ParseBool(kv.Get(config.Enable))
		if err != nil {
			return cfg, err
		}
		if !enabled {
			continue
		}
		err = config.EnsureCertAndKey(kv.Get(ClientCert), kv.Get(ClientKey))
		if err != nil {
			return cfg, err
		}
		queueSize, err := strconv.Atoi(kv.Get(QueueSize))
		if err != nil {
			return cfg, err
		}
		if queueSize <= 0 {
			return cfg, errors.New("invalid queue_size value")
		}
		cfg.HTTP[starget] = http.Config{
			Enabled:    true,
			Endpoint:   kv.Get(Endpoint),
			AuthToken:  kv.Get(AuthToken),
			ClientCert: kv.Get(ClientCert),
			ClientKey:  kv.Get(ClientKey),
			Proxy:      kv.Get(Proxy),
			QueueSize:  queueSize,
			QueueDir:   kv.Get(QueueDir),
			Name:       loggerTargetNamePrefix + starget,
		}
	}

	return cfg, nil
}

func lookupAuditWebhookConfig(scfg config.Config, cfg Config) (Config, error) {
	var loggerAuditTargets []string
	envs := env.List(EnvAuditWebhookEndpoint)
	for _, k := range envs {
		target := strings.TrimPrefix(k, EnvAuditWebhookEndpoint+config.Default)
		if target == EnvAuditWebhookEndpoint {
			target = config.Default
		}
		loggerAuditTargets = append(loggerAuditTargets, target)
	}

	for _, target := range loggerAuditTargets {
		if v, ok := cfg.AuditWebhook[target]; ok && v.Enabled {
			// This target is already enabled using the
			// legacy environment variables, ignore.
			continue
		}
		enable, err := config.ParseBool(getCfgVal(EnvAuditWebhookEnable, target, ""))
		if err != nil || !enable {
			continue
		}

		clientCert := getCfgVal(EnvAuditWebhookClientCert, target, "")
		clientKey := getCfgVal(EnvAuditWebhookClientKey, target, "")
		err = config.EnsureCertAndKey(clientCert, clientKey)
		if err != nil {
			return cfg, err
		}

		queueSizeCfgVal := getCfgVal(EnvAuditWebhookQueueSize, target, "100000")
		queueSize, err := strconv.Atoi(queueSizeCfgVal)
		if err != nil {
			return cfg, err
		}
		if queueSize <= 0 {
			return cfg, errors.New("invalid queue_size value")
		}

		cfg.AuditWebhook[target] = http.Config{
			Enabled:    true,
			Endpoint:   getCfgVal(EnvAuditWebhookEndpoint, target, ""),
			AuthToken:  getCfgVal(EnvAuditWebhookAuthToken, target, ""),
			ClientCert: clientCert,
			ClientKey:  clientKey,
			QueueSize:  queueSize,
			QueueDir:   getCfgVal(EnvAuditWebhookQueueDir, target, ""),
			Name:       auditTargetNamePrefix + target,
		}
	}

	for starget, kv := range scfg[config.AuditWebhookSubSys] {
		if l, ok := cfg.AuditWebhook[starget]; ok && l.Enabled {
			// Ignore this audit config since another target
			// with the same name is already loaded and enabled
			// in the shell environment.
			continue
		}
		subSysTarget := config.AuditWebhookSubSys
		if starget != config.Default {
			subSysTarget = config.AuditWebhookSubSys + config.SubSystemSeparator + starget
		}
		if err := config.CheckValidKeys(subSysTarget, kv, DefaultAuditWebhookKVS); err != nil {
			return cfg, err
		}
		enabled, err := config.ParseBool(kv.Get(config.Enable))
		if err != nil {
			return cfg, err
		}
		if !enabled {
			continue
		}
		err = config.EnsureCertAndKey(kv.Get(ClientCert), kv.Get(ClientKey))
		if err != nil {
			return cfg, err
		}
		queueSize, err := strconv.Atoi(kv.Get(QueueSize))
		if err != nil {
			return cfg, err
		}
		if queueSize <= 0 {
			return cfg, errors.New("invalid queue_size value")
		}
		cfg.AuditWebhook[starget] = http.Config{
			Enabled:    true,
			Endpoint:   kv.Get(Endpoint),
			AuthToken:  kv.Get(AuthToken),
			ClientCert: kv.Get(ClientCert),
			ClientKey:  kv.Get(ClientKey),
			QueueSize:  queueSize,
			QueueDir:   kv.Get(QueueDir),
			Name:       auditTargetNamePrefix + starget,
		}
	}

	return cfg, nil
}

// LookupConfigForSubSys - lookup logger config, override with ENVs if set, for the given sub-system
func LookupConfigForSubSys(scfg config.Config, subSys string) (cfg Config, err error) {
	switch subSys {
	case config.LoggerWebhookSubSys:
		cfg = lookupLegacyConfigForSubSys(config.LoggerWebhookSubSys)
		if cfg, err = lookupLoggerWebhookConfig(scfg, cfg); err != nil {
			return cfg, err
		}
	case config.AuditWebhookSubSys:
		cfg = lookupLegacyConfigForSubSys(config.AuditWebhookSubSys)
		if cfg, err = lookupAuditWebhookConfig(scfg, cfg); err != nil {
			return cfg, err
		}
	case config.AuditKafkaSubSys:
		cfg.AuditKafka = make(map[string]kafka.Config)
		if cfg, err = lookupAuditKafkaConfig(scfg, cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

// ValidateSubSysConfig - validates logger related config of given sub-system
func ValidateSubSysConfig(scfg config.Config, subSys string) error {
	// Lookup for legacy environment variables first
	_, err := LookupConfigForSubSys(scfg, subSys)
	return err
}
