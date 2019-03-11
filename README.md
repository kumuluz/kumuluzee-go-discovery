# KumuluzEE Go Discovery

KumuluzEE Go Discovery is a service discovery library for the KumuluzEE microservice framework. It is a Go package based on a [KumuluzEE Discovery](https://github.com/kumuluz/kumuluzee-discovery), service discovery extension for microservices written in Java programming language. It provides support for service registration, service discovery and client side load balancing.

KumuluzEE Go Discovery provides full support for microservices packed as Docker containers. It also provides full support for executing microservices in clusters and cloud-native platforms with full support for Kubernetes.

## Install

You can `go get` this package:

```
$ go get github.com/kumuluz/kumuluzee-go-discovery/discovery
```

Note that `kumuluzee-go-discovery` depends on [kumuluzee-go-config/config](https://github.com/kumuluz/kumuluzee-go-config) package, therefore when we `go get` discovery package, config package is downloaded as well.

## Setup

Before you can start using this library you should configure properties in order to successfully connect to desired discovery framework. If you wish to connect to Consul check section [Configuring Consul](https://github.com/kumuluz/kumuluzee-discovery#configuring-consul) or [Configuring etcd](https://github.com/kumuluz/kumuluzee-discovery#configuring-etcd) to connect to etcd.

Library also supports retry delays on watch connection errors. For more information check [Retry delays](https://github.com/kumuluz/kumuluzee-discovery#retry-delays).

## Usage

### discovery.Util

*discovery.New(options)*

Connect to a given discovery source. Function accepts `discovery.Options` struct with following fields:
* **Extension** (string): name of service discovery source, possible values are "consul" and "etcd" 
* **ConfigPath** (string): path to configuration source file, defaults to "config/config.yaml"

Example usage:

```go
// import package
import "github.com/kumuluz/kumuluzee-go-discovery/discovery"

// usage
var disc discovery.Util

disc = discovery.New(discovery.Options(
    Extension: "consul",
})
```

***.RegisterService(options)***

Registers service to specified discovery source with given options.

Function accepts `discovery.RegisterOptions` struct with following fields:
* **Value** (string): service name of a registered service. Service name can be overridden with configuration key  `kumuluzee.name`,
* **TTL** (integer): seconds to live of a registration key in the store. Default value is `30`. TTL can be overridden with configuration key `kumuluzee.discovery.ttl`,
* **PingInterval** (integer): an interval in which service updates registration key value in the store. Default value is `20` seconds. Ping interval can be overridden with configuration key  `kumuluzee.discovery.ping-interval`,
* **Environment** (string): environment in which service is registered. Default value is `'dev'`. Environment can be overridden with configuration key  `kumuluzee.env.name`,
* **Version** (string): version of service to be registered. Default value is `'1.0.0'`. Version can be overridden with configuration key  `kumuluzee.version`,
* **Singleton** (boolean): if true ensures, that only one instance of service with the same name, version and environment is registered. Default value is `false`.

Example of service registration:

```go
disc.RegisterService(discovery.RegisterOptions{
    Value: "my-service",
    TTL: 40,
    PingInterval: 20,
    Environment: "test",
    Version: "1.1.0",
    Singleton: false,
})
```

To register a service with etcd, service URL has to be provided with the configuration key `kumuluzee.server.base-url` in the following format: `http://localhost:8080`.
Consul implementation uses agent's IP address for the URL of registered services.

***.DeregisterService()***

Deregisters service from the service registry. Service deregistration needs to be performed manually, for example when service receives a terminate signal (SIGTERM):

```go
// catch interrupt or terminate signals and send them to sigs channel
sigs := make(chan os.Signal, 1)
signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

// function waits for received signal - and then performs service deregistration
go func() {
    <-sigs
    if err := disc.DeregisterService(); err != nil {
        panic(err)
    }
    // Make sure to call os.Exit() with status number at the end.
    os.Exit(1)
}()
```

See [discovery sample in kumuluzee-go-samples](https://github.com/kumuluz/kumuluzee-go-samples/tree/master/kumuluzee-go-discovery) for example of service deregistration upon receiving interrupt or terminate signals.

***.DiscoverService(options)***

Discovers service on specified discovery source.

Function takes four parameters:

* **value** (string): name of the service we want to discover,
* **environment** (string): service environment, e.g. prod, dev, test. If value is not provided, environment is set to the value defined with the configuration key  `kumuluzee.env.name`. If the configuration key is not present, value is set to  `'dev'`,
* **version** (string): service version or NPM version range. Default value is `'*'`, which resolves to the highest deployed version,
* **accessType** (string): defines, which URL is returned. Supported values are  `'GATEWAY'`  and  `'DIRECT'`. Default is  `'GATEWAY'`.

Example of service discovery:

```go
serviceURL, err := disc.DiscoverService(discovery.DiscoverOptions{
    Value:       "",
    Environment: "dev",
    Version:     "*",
    AccessType:  discovery.AccessTypeDirect
})

if err != nil {
    // There was an error, therefore no service was discovered
    fmt.Printf("No service discovered, error: %s\n", err.Error())

} else {
    // There was no error, a service was discovered 
    fmt.Printf("Service discovered, address: %s\n", serviceURL)

}
```

**Access types**

Service discovery supports two access types:

*   `GATEWAY`  returns gateway URL, if it is present. If not, behavior is the same as with  `DIRECT`,
*   `DIRECT`  always returns base URL or container URL.

If etcd implementation is used, gateway URL is read from etcd key-value store used for service discovery. It is stored in key `/environments/'environment'/services/'serviceName'/'serviceVersion'/gatewayUrl` and is automatically updated, if value changes.

If Consul implementation is used, gateway URL is read from Consul key-value store. It is stored in key `/environments/'environment'/services/'serviceName'/'serviceVersion'/gatewayUrl`  and is automatically updated on changes.

**NPM-like versioning**

Service discovery supports semantic versioning. If service is registered with version in proper semantic version format, it can be discovered using a semantic version range. Service parsing is done using [blang/semver package](https://github.com/blang/semver). How to input ranges and other possible inputs are available in [package's README](https://github.com/blang/semver/blob/master/README.md). NPM-like ranges using `^` and `~` are also supported. Some examples:

-   `'^1.0.4'` would discover the latest minor version (equal to range `>=1.0.4 <2.0.0`)
-   `'~1.0.4'` would discover the latest patch version (equal to range `>=1.0.4 <1.1.0`)

For more information see  [Semantic versioning spec](https://semver.org/).

### Cluster, cloud-native platforms and Kubernetes
KumuluzEE Go Discovery is also fully compatible with clusters and cloud-native platforms. For more information check [Cluster, cloud-native platforms and Kubernetes](https://github.com/kumuluz/kumuluzee-discovery#cluster-cloud-native-platforms-and-kubernetes).

## Changelog

Recent changes can be viewed on Github on the  [Releases Page](https://github.com/kumuluz/kumuluzee/releases)

## Contribute

See the  [contributing docs](https://github.com/kumuluz/kumuluzee-go-discovery/blob/master/CONTRIBUTING.md)

When submitting an issue, please follow the  [guidelines](https://github.com/kumuluz/kumuluzee-go-discovery/blob/master/CONTRIBUTING.md#bugs).

When submitting a bugfix, write a test that exposes the bug and fails before applying your fix. Submit the test alongside the fix.

When submitting a new feature, add tests that cover the feature.

## License

MIT
