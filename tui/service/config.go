package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type EngineConfig struct {
	Endpoint   string `json:"endpoint"`
	ServerName string `json:"server_name"`
	TLSDir     string `json:"tls_dir"`
	Command    string `json:"command"`
}

type Config struct {
	APIListen       string       `json:"api_listen"`
	GRPCListen      string       `json:"grpc_listen,omitempty"`
	ProbeListen     string       `json:"probe_listen"`
	Workspace       string       `json:"workspace"`
	StateDir        string       `json:"state_dir"`
	ServerCertFile  string       `json:"server_cert_file"`
	ServerKeyFile   string       `json:"server_key_file"`
	ClientCAFile    string       `json:"client_ca_file"`
	PrincipalsFile  string       `json:"principals_file"`
	ModelAPIKeyFile string       `json:"model_api_key_file"`
	KMP             EngineConfig `json:"kmp"`
	MADE            EngineConfig `json:"made"`
}

func LoadConfig(path string) (Config, error) {
	if !filepath.IsAbs(path) {
		return Config{}, errors.New("config path must be absolute")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	if len(data) > 64<<10 {
		return Config{}, errors.New("config exceeds 64 KiB")
	}
	var cfg Config
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return Config{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Config{}, errors.New("trailing config data")
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if c.APIListen == "" || c.ProbeListen == "" {
		return errors.New("API and probe listeners are required")
	}
	probeHost, _, err := net.SplitHostPort(c.ProbeListen)
	if err != nil || net.ParseIP(probeHost) == nil || !net.ParseIP(probeHost).IsLoopback() {
		return errors.New("probe listener must bind loopback")
	}
	if _, _, err := net.SplitHostPort(c.APIListen); err != nil {
		return errors.New("invalid API listener")
	}
	if c.GRPCListen != "" {
		if _, _, err := net.SplitHostPort(c.GRPCListen); err != nil {
			return errors.New("invalid gRPC listener")
		}
		if c.GRPCListen == c.APIListen || c.GRPCListen == c.ProbeListen {
			return errors.New("gRPC listener must be separate from API and probes")
		}
	}
	for name, value := range map[string]string{"workspace": c.Workspace, "state_dir": c.StateDir, "server_cert_file": c.ServerCertFile, "server_key_file": c.ServerKeyFile, "client_ca_file": c.ClientCAFile, "principals_file": c.PrincipalsFile, "model_api_key_file": c.ModelAPIKeyFile} {
		if !filepath.IsAbs(value) || strings.ContainsRune(value, 0) {
			return fmt.Errorf("%s must be an absolute path", name)
		}
	}
	for name, engine := range map[string]EngineConfig{"kmp": c.KMP, "made": c.MADE} {
		if engine.Endpoint == "" || engine.ServerName == "" || !filepath.IsAbs(engine.TLSDir) || !filepath.IsAbs(engine.Command) {
			return fmt.Errorf("%s requires endpoint, server name, TLS directory and command", name)
		}
		if _, _, err := net.SplitHostPort(engine.Endpoint); err != nil {
			return fmt.Errorf("%s endpoint must be host:port", name)
		}
	}
	return nil
}
