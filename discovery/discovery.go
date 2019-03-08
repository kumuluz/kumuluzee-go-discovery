/*
 *  Copyright (c) 2019 Kumuluz and/or its affiliates
 *  and other contributors as indicated by the @author tags and
 *  the contributor list.
 *
 *  Licensed under the MIT License (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  https://opensource.org/licenses/MIT
 *
 *  The software is provided "AS IS", WITHOUT WARRANTY OF ANY KIND, express or
 *  implied, including but not limited to the warranties of merchantability,
 *  fitness for a particular purpose and noninfringement. in no event shall the
 *  authors or copyright holders be liable for any claim, damages or other
 *  liability, whether in an action of contract, tort or otherwise, arising from,
 *  out of or in connection with the software or the use or other dealings in the
 *  software. See the License for the specific language governing permissions and
 *  limitations under the License.
 */

// Package discovery provides service discovery for the KumuluzEE microservice framework.
package discovery

import (
	"github.com/kumuluz/kumuluzee-go-config/config"
	"github.com/mc0239/logm"
)

// Options struct is used when instantiating a new Util.
type Options struct {
	// Additional configuration source to connect to. Possible values are: "consul", "etcd"
	Extension string
	// ConfigPath is a path to configuration file, including the configuration file name.
	// Passing an empty string will default to config/config.yaml
	ConfigPath string
	// LogLevel can be used to limit the amount of logging output. Default log level is 0. Level 4
	// will only output Warnings and Errors, and level 5 will only output errors.
	// See package github.com/mc0239/logm for more details on logging and log levels.
	LogLevel int
}

// RegisterOptions is used when registering a service
type RegisterOptions struct {
	// Service name to register the service by.
	// Can be overridden with configuration key kumuluzee.name
	Value string
	// Time to live of a registration key in the store (in seconds).
	// Default value is 30.
	// Can be overridden with configuration key kumuluzee.discovery.ttl
	TTL int64
	// Interval in which service updates registration key value in store.
	// Default value is 20.
	// Can be overridden with configuration key kumuluzee.discovery.ping-interval
	PingInterval int64
	// Environment in which the service is registered.
	// Default value is "dev".
	// Can be overridden with configuration key kumuluzee.env.name
	Environment string
	// Version of the service to be registered.
	// Default value is "1.0.0".
	// Can be overridden with configuration key kumuluzee.version
	Version string
	// If set to true, only once instance of service with the same name, version and environment is registered.
	// Default value is false.
	Singleton bool
}

// DiscoverOptions is used when discovering services
type DiscoverOptions struct {
	// Name of the service to discover.
	Value string
	// Environment of the service to discover.
	// If value is not provided, it uses value from configuration with key kumuluzee.env.name
	// If value is not specified and key in configuration does not exists, value defaults to 'dev'.
	Environment string
	// Version of the service to discover.
	// Supported values are semantic version (semver) parseable versions/version ranges.
	// Default value is "*", which resolves to highest deployed version.
	Version string
	// AccessType defines, which URL gets injected.
	// Supported values are constants discovery.AccessTypeGateway and discovery.AccessTypeDirect.
	// Default value is discovery.AccessTypeGateway.
	AccessType string
}

// Possible access types for DiscoverOptions.AccessType
const (
	AccessTypeDirect  = "direct"
	AccessTypeGateway = "gateway"
)

// Util is used for registering and discovering services from a service discovery source.
// Util should be initialized with discovery.New() function
type Util struct {
	discoverySource discoverySource
	Logger          logm.Logm
}

type discoverySource interface {
	RegisterService(options RegisterOptions) (serviceID string, err error)
	DeregisterService() error
	DiscoverService(options DiscoverOptions) (string, error)
}

// New instantiates Util struct with initialized service discovery
func New(options Options) Util {

	lgr := logm.New("KumuluzEE-discovery")
	lgr.LogLevel = options.LogLevel

	var src discoverySource

	if options.Extension == "consul" {
		// TODO: potential mixup between cofig.Options and (discovery.)Options
		src = newConsulDiscoverySource(config.Options{
			Extension:  options.Extension,
			ConfigPath: options.ConfigPath,
			LogLevel:   options.LogLevel,
		}, &lgr)
	} else if options.Extension == "etcd" {
		src = newEtcdDiscoverySource(config.Options{
			Extension:  options.Extension,
			ConfigPath: options.ConfigPath,
			LogLevel:   options.LogLevel,
		}, &lgr)
	} else {
		lgr.Error("Specified discovery source extension is invalid.")
	}

	k := Util{
		src,
		lgr,
	}

	return k
}

// RegisterService registers service using service discovery client with given RegisterOptions
func (d Util) RegisterService(options RegisterOptions) (string, error) {
	return d.discoverySource.RegisterService(options)
}

// DeregisterService removes service from the registry (deregisters).
func (d Util) DeregisterService() error {
	return d.discoverySource.DeregisterService()
}

// DiscoverService discovery services using service discovery client with given RegisterOptions
func (d Util) DiscoverService(options DiscoverOptions) (string, error) {
	return d.discoverySource.DiscoverService(options)
}
