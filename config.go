package main

import (
	log "github.com/sirupsen/logrus"
	"go.mozilla.org/sops/v3"
	"go.mozilla.org/sops/v3/decrypt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type KafkaConfig struct {
	Brokers []string `yaml:"bootstrapBrokers"`
	SSL     struct {
		Enabled    bool   `yaml:"enabled"`
		CA         string `yaml:"caPath"`
		SkipVerify bool   `yaml:"skipVerify"`
	} `yaml:"ssl"`
	Krb5 struct {
		Enabled            bool   `yaml:"enabled"`
		Keytab             string `yaml:"keytab"`
		ServiceName        string `yaml:"serviceName"`
		Realm              string `yaml:"realm"`
		Username           string `yaml:"username"`
		Password           string `yaml:"password"`
		KerberosConfigPath string `yaml:"krb5Path"`
	} `yaml:"kerberos"`
	Producer struct {
		MaxMessageBytes int    `yaml:"maxMessageBytes"`
		Compression     string `yaml:"compression"`
	} `yaml:"producer"`
}
type MDSConfig struct {
	Url                     string `yaml:"url"`
	User                    string `yaml:"username"`
	Password                string `yaml:"password"`
	SchemaRegistryClusterID string `yaml:"schema-registry-cluster-id"`
	ConnectClusterId        string `yaml:"connect-cluster-id"`
	KSQLClusterID           string `yaml:"ksql-cluster-id"`
	CAPath                  string `yaml:"caPath"` // Add a trusted CA
	SkipVerify              bool   `yaml:"skipVerify"`
}

type ConnectConfig struct {
	Url        string `yaml:"url"`
	User       string `yaml:"username"`
	Password   string `yaml:"password"`
	CAPath     string `yaml:"caPath"` // Add a trusted CA
	SkipVerify bool   `yaml:"skipVerify"`
}
type SRConfig struct {
	Url        string        `yaml:"url"`
	Timeout    time.Duration `yaml:"timeout"` // Allow setting custom timeout for API calls
	Username   string        `yaml:"username"`
	Password   string        `yaml:"password"`
	CAPath     string        `yaml:"caPath"` // Add a trusted CA
	SkipVerify bool          `yaml:"skipVerify"`
	// When this is true, Gafkalo will read the _schemas topic directly and
	// use an internal cache for read operations, bypassign the REST API
	SkipRestForReads bool `yaml:"skipRegistryForReads"`
}
type Configuration struct {
	Connections struct {
		Kafka          KafkaConfig   `yaml:"kafka"`
		Schemaregistry SRConfig      `yaml:"schemaregistry"`
		Mds            MDSConfig     `yaml:"mds"`
		Connect        ConnectConfig `yaml:"connect"`
	} `yaml:"connections"`
	Kafkalo struct {
		InputDirs                    []string `yaml:"input_dirs"`
		SchemaDir                    string   `yaml:"schema_dir"` // Directory to look for schemas when using a relative path
		ConnectorsSensitiveKeysRegex string   `yaml:"connectors_sensitive_keys"`
	} `yaml:"kafkalo"`
}

func parseConfig(configFile string) Configuration {
	var configData, data []byte
	var err error
	data, err = ioutil.ReadFile(configFile)
	if err != nil {
		log.Printf("unable to read %s with error %s\n", configFile, err)
	}
	configData, err = decrypt.Data(data, "yaml")
	/* try to decrypt using sops.
	If we have an error MetadataNotFound, then we consider the file plaintext and ignore this error
	*/
	if err != nil && err != sops.MetadataNotFound {
		log.Fatalf("Failed to read config: %s", err)
	} else if err == sops.MetadataNotFound {
		configData = data
	}

	var Config Configuration
	err = yaml.Unmarshal(configData, &Config)
	if err != nil {
		log.Fatalf("Failed to read kafkalo config: %s\n", err)
	}
	return Config

}

// Get the input patterns to use
func (conf *Configuration) GetInputPatterns() []string {
	return conf.Kafkalo.InputDirs
}

// Resolve input patterns to actual files to read (expand globs and list files)
func (conf *Configuration) ResolveFilesFromPatterns(patterns []string) ([]string, error) {
	var files []string
	for _, pattern := range patterns {
		// Check if we have a Glob pattern or a direct file
		if strings.Contains(pattern, "*") {
			// We have a glob pattern
			matches, err := filepath.Glob(pattern)
			// Filter out unwanted matches (like directories)
			for _, match := range matches {
				if isValidInputFile(match) {
					files = append(files, match)
				}
			}
			if err != nil {
				log.Fatalf("Could not read match pattern: %s\n", err)
			}
		} else {
			if isValidInputFile(pattern) {
				files = append(files, pattern)
			}
		}
	}
	return files, nil
}

// Takes a schema path as specified in the yaml and returns a "normalized" path
// If it is an absolute path, it is returned as is
// If it is a relative path it is made relative to the source dir
func normalizeSchemaPath(inputFile string) string {
	var res string
	if gafkaloConfig.Kafkalo.SchemaDir != "" {
		res = path.Join(gafkaloConfig.Kafkalo.SchemaDir, inputFile)
	} else {
		res = inputFile
	}
	return res
}

// Validates an input filename as valid (exists, is not dir etc)
func isValidInputFile(filename string) bool {
	fi, err := os.Stat(filename)
	if err != nil {
		//log.Printf("Ignoring file %s due to: %s\n", filename, err)
		return false
	}
	if !fi.Mode().IsRegular() {
		//log.Printf("Ignoring file %s because its not regular\n", filename)
		return false
	}
	return true
}
