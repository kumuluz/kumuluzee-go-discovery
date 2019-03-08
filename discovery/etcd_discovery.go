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

package discovery

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/blang/semver"

	"github.com/kumuluz/kumuluzee-go-config/config"
	"github.com/mc0239/logm"
	uuid "github.com/satori/go.uuid"
	"go.etcd.io/etcd/client"
)

// holds etcd client instance and configuration
type etcdDiscoverySource struct {
	client   *client.Client
	kvClient client.KeysAPI

	startRetryDelay int64
	maxRetryDelay   int64

	configOptions   config.Options         // passed when calling new...()
	options         *registerConfiguration // loaded as config bundle
	serviceInstance *etcdServiceInstance

	lastKnownService string // last known service from discovery
	gatewayURLs      []*gatewayURLWatch

	logger *logm.Logm
}

// holds service instance configuration and state
type etcdServiceInstance struct {
	isRegistered bool

	id         string
	etcdKeyDir string
	serviceURL string

	singleton bool
}

func newEtcdDiscoverySource(options config.Options, logger *logm.Logm) discoverySource {
	var d etcdDiscoverySource
	logger.Verbose("Initializing etcd discovery source")
	d.logger = logger

	d.configOptions = options
	conf := config.NewUtil(config.Options{
		ConfigPath: options.ConfigPath,
		LogLevel:   logm.LvlWarning, // bit less logs from config
	})

	startRD, maxRD := getRetryDelays(conf)
	d.startRetryDelay = startRD
	d.maxRetryDelay = maxRD
	logger.Verbose("start-retry-delay-ms=%d, max-retry-delay-ms=%d", d.startRetryDelay, d.maxRetryDelay)

	var etcdAddresses string
	if addr, ok := conf.GetString("kumuluzee.discovery.etcd.hosts"); ok {
		etcdAddresses = addr
	} else {
		etcdAddresses = "http://localhost:2379"
	}
	if client, err := createEtcdClient(etcdAddresses); err == nil {
		logger.Info("etcd client addresses set to: %v", etcdAddresses)
		d.client = client
	} else {
		logger.Error("Failed to create etcd client: %s", err.Error())
	}

	d.kvClient = client.NewKeysAPI(*d.client)

	return &d
}

func (d *etcdDiscoverySource) RegisterService(options RegisterOptions) (serviceID string, err error) {
	regconf := loadServiceRegisterConfiguration(d.configOptions, options)
	d.options = &regconf

	d.serviceInstance = &etcdServiceInstance{
		singleton: options.Singleton,
	}

	uuid4, err := uuid.NewV4()
	if err != nil {
		d.logger.Error(err.Error())
	}

	d.serviceInstance.id = uuid4.String()

	d.serviceInstance.etcdKeyDir = fmt.Sprintf("/environments/%s/services/%s/%s/instances/%s",
		regconf.Env.Name, regconf.Name, regconf.Version, d.serviceInstance.id)

	go d.run(d.startRetryDelay)

	return d.serviceInstance.id, nil
}

func (d *etcdDiscoverySource) DeregisterService() error {
	d.logger.Info("Service deregistration, id=%s", d.serviceInstance.id)
	_, err := d.kvClient.Delete(context.Background(),
		d.serviceInstance.etcdKeyDir,
		&client.DeleteOptions{
			Recursive: true,
			Dir:       true,
		})
	return err
}

func (d *etcdDiscoverySource) DiscoverService(options DiscoverOptions) (string, error) {
	fillDefaultDiscoverOptions(&options)

	kvPath := fmt.Sprintf("environments/%s/services/%s/", options.Environment, options.Value)

	resp, err := d.kvClient.Get(context.Background(), kvPath, &client.GetOptions{
		Recursive: true,
	})

	if err != nil {
		if d.lastKnownService != "" {
			d.logger.Warning("Service discovery failed, using last known service. Error: %s", err.Error())
			return d.lastKnownService, nil
		}
		d.logger.Error("Service discovery failed: %s", err.Error())
		return "", err
	}

	// ----- extract all services of all versions of given environment and name
	var discoveredInstances []discoveredService
	// iterate all versions
	for _, nodeVersion := range resp.Node.Nodes {
		currentVersion := path.Base(nodeVersion.Key)
		// we need .../instances/ key
		var instances *client.Node
		for _, n := range nodeVersion.Nodes {
			if path.Base(n.Key) == "instances" {
				instances = n
				break
			}
		}

		// iterate all instances
		for _, instance := range instances.Nodes {
			discoveredInstance := discoveredService{}
			discoveredInstance.id = path.Base(instance.Key)

			version, err := semver.ParseTolerant(currentVersion)
			if err != nil {
				d.logger.Warning("semver parsing failed for: %s, error: %s", currentVersion, err.Error())
				break // break out of this version, can't parse it
			}
			discoveredInstance.version = version

			for _, node := range instance.Nodes {
				// fmt.Printf("key=%v value=%v", node.Key, node.Value)
				if path.Base(node.Key) == "url" {
					discoveredInstance.directURL = node.Value
				}
			}

			discoveredInstances = append(discoveredInstances, discoveredInstance)

			// ---- add a watch for gatewayUrl for discovering service (if not already made)
			// TODO: this part is the same for both etcd & consul: make the code more DRY
			watcherNamespace := fmt.Sprintf("/environments/%s/services/%s/%s", options.Environment, options.Value, discoveredInstance.version.String())

			util := config.NewUtil(config.Options{
				Extension:          d.configOptions.Extension,
				ExtensionNamespace: watcherNamespace,
				ConfigPath:         d.configOptions.ConfigPath,
				LogLevel:           logm.LvlMute,
			})

			var hasWatch bool
			for _, w := range d.gatewayURLs {
				if w.gatewayID == watcherNamespace {
					// watch already set :)
					hasWatch = true
					break
				}
			}
			if !hasWatch {
				// make a watch for this one!
				d.logger.Info("Creating a gatewayUrl watch for %s", watcherNamespace)

				g, _ := util.GetString("gatewayUrl")
				d.gatewayURLs = append(d.gatewayURLs, &gatewayURLWatch{
					gatewayID:  watcherNamespace,
					gatewayURL: g,
				})
				util.Subscribe("gatewayUrl", func(key string, value string) {
					for _, w := range d.gatewayURLs {
						if w.gatewayID == watcherNamespace {
							d.logger.Info("Updated gatewayUrl value for %s (new value: %s)", watcherNamespace, value)
							w.gatewayURL = value
							break
						}
					}
					return
				})
			}
			// ----
		}
	}
	// -----

	service, err := pickRandomServiceInstance(discoveredInstances, d.gatewayURLs, options, d.lastKnownService)

	if err != nil {
		if service != "" {
			d.logger.Warning("Service discovery failed, using last known service. Error: %s", err.Error())
			return d.lastKnownService, nil
		}

		d.logger.Error("Service discovery failed: %s", err.Error())
		return "", err
	}

	d.lastKnownService = service
	return service, nil
}

// functions that aren't discoverySource methods

// if service is not registered, performs registration. Otherwise perform ttl update
func (d *etcdDiscoverySource) run(retryDelay int64) {

	var ok bool
	if !d.serviceInstance.isRegistered {
		ok = d.register(retryDelay)
		if ok {
			d.serviceInstance.isRegistered = true
		}
	} else {
		ok = d.ttlUpdate(retryDelay)
		if !ok {
			d.serviceInstance.isRegistered = false
		}
	}

	if !ok {
		// Something went wrong with either registration or TTL update :(

		// sleep for current delay
		time.Sleep(time.Duration(retryDelay) * time.Millisecond)
		// exponentially extend retry delay, but keep it at most maxRetryDelay
		newRetryDelay := retryDelay * 2
		if newRetryDelay > d.maxRetryDelay {
			newRetryDelay = d.maxRetryDelay
		}
		d.run(newRetryDelay)
	} else {
		// Everything is alright, either registration or TTL update was successful :)

		time.Sleep(time.Duration(d.options.Discovery.PingInterval) * time.Second)
		d.run(d.startRetryDelay)
	}

}

func (d *etcdDiscoverySource) register(retryDelay int64) bool {
	inst := d.serviceInstance

	if d.isServiceRegistered() && inst.singleton {
		d.logger.Error("Service of this kind is already registered, not registering with options.singleton set to true")
		return false
	}

	d.logger.Info("Registering service: id=%s address=%s port=%d", inst.id, d.options.Server.HTTP.Address, d.options.Server.HTTP.Port)

	d.serviceInstance.serviceURL = d.options.Server.BaseURL
	if d.serviceInstance.serviceURL == "" {
		// TODO: if base-url not defined, assume URL from system network interface?
		d.logger.Error("No base-url provided! Please provide base-url by setting a key kumuluzee.server.base-url in your configuration!")
	}

	// set TTL on instance directory
	_, err := d.kvClient.Set(context.Background(),
		d.serviceInstance.etcdKeyDir,
		"",
		&client.SetOptions{
			TTL: time.Duration(d.options.Discovery.TTL) * time.Second,
			Dir: true,
		})
	if err != nil {
		d.logger.Error(fmt.Sprintf("Service registration failed: %s", err.Error()))
		return false
	}

	_, err = d.kvClient.Set(context.Background(),
		d.serviceInstance.etcdKeyDir+"/url",
		d.serviceInstance.serviceURL,
		nil)
	if err != nil {
		d.logger.Error(fmt.Sprintf("Service registration failed: %s", err.Error()))
		return false
	}

	d.logger.Info("Service registered, id=%s", inst.id)
	return true
}

func (d *etcdDiscoverySource) ttlUpdate(retryDelay int64) bool {
	inst := d.serviceInstance
	// d.logger.Verbose("Updating TTL for service %s", inst.id)

	_, err := d.kvClient.Set(context.Background(), inst.etcdKeyDir, "", &client.SetOptions{
		TTL:       time.Duration(d.options.Discovery.TTL) * time.Second,
		Dir:       true,
		PrevExist: client.PrevExist,
		Refresh:   true,
	})

	if err != nil {
		d.logger.Error("TTL update failed, error: %s, retry delay: %d ms", inst.id, err.Error(), retryDelay)
		return false
	}

	d.logger.Verbose("TTL update for service %s", inst.id)
	return true
}

// returns true if there are any services of this kind (env+name) registered
func (d *etcdDiscoverySource) isServiceRegistered() bool {
	etcdKeyDir := fmt.Sprintf("/environments/%s/services/%s/%s/instances/",
		d.options.Env.Name, d.options.Name, d.options.Version)

	resp, err := d.kvClient.Get(context.Background(), etcdKeyDir, &client.GetOptions{
		Recursive: true,
	})

	if err != nil {
		d.logger.Warning("isServiceRegistered() failed: %s", err.Error())
		return false
	}

	for _, instance := range resp.Node.Nodes {
		var URL string
		var isActive = true

		for _, node := range instance.Nodes {
			if path.Base(node.Key) == "url" {
				URL = node.Value
			}
			if path.Base(node.Key) == "status" {
				if node.Value == "disabled" {
					isActive = false
				}
			}
		}

		if isActive && URL != "" {
			return true
		}
	}

	return false
}

// functions that aren't discoverySource methods or etcdDiscoverySource methods

func createEtcdClient(addresses string) (*client.Client, error) {
	clientConfig := client.Config{
		Endpoints: strings.Split(addresses, ","),
	}

	client, err := client.New(clientConfig)
	if err != nil {
		return nil, err
	}
	return &client, nil
}
