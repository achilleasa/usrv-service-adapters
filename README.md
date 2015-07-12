# service-adapters

This package is essentially a support package for [usrv](https://github.com/achilleasa/usrv)
and related-packages although it can also be used standalone. It provides a common interface
for configuring and instanciating services such as redis and rabbitmq.

Features:

- Common [interface](https://github.com/achilleasa/service-adapters/blob/master/service.go) for managing and configuring services
- Dial policies (periodic, exp. backoff or user-defined)
- Modular configuration (etcd plugin or plain maps)
- Service close notifications
- Thread-safe implementation

# Required dependencies

| Service | Required depenencies |
|---------|----------------------|
| redis   | ```go get github.com/garyburd/redigo/redis```
| rabbitmq| ```go get github.com/streadway/amqp```
| etcd    | ```go get github.com/coreos/go-etcd``` ```go get github.com/ugorji/go/codec```



# Service close notifications

You may register one or more 

# Service options

You can apply zero or more service options when instanciating a service using its `New()` method. All
options follow the [functional arguments](http://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html) pattern.

## Config

`Config` allows you to specify a map with configuration settings during the service instanciation. The
service adapters will cause the service to reset whenever the settings change. You may also change
the config settings after the service has been instanciated using its `Config` method. 

Example usage:

```go
package main

import (
	"github.com/achilleasa/service-adapters/service/redis"
	"github.com/achilleasa/service-adapters"
)

func setup() *redis.Redis {
	opts := map[string]string{
		"endpoint" : "localhost:6379",
	}

	redisSrv, err := redis.New(
		adapters.Config(opts),
	)
	if err != nil {
		panic(err)
	}

	return redisSrv
}
```


## Logger

`Logger` allows you to attach a specific [Logger](http://golang.org/pkg/log/) instance to an instanciated service.
By default, all services are provisioned with a /dev/null logger that discards all output. Alternatively, you can
attach the logger after the service has been instanciated using its `SetLogger` method.

Example usage:

```go
package main

import (
	"log"
	"os"

	"github.com/achilleasa/service-adapters"
	"github.com/achilleasa/service-adapters/service/redis"
)

func setup() *redis.Redis {
	logger := log.New(os.Stderr, "[Custom Logger]", log.LstdFlags)

	redisSrv, err := redis.New(
		adapters.Logger(logger),
	)
	if err != nil {
		panic(err)
	}

	return redisSrv
}
```

## DialPolicy

The `DialPolicy` option allows you to specify the policy for dialing each service. Selecting the appropriate policy for a service ensures that adaptor instances do not hammer on the remote endpoints whenever the connection is lost/dropped.

A dial policy is essentially a generator of retry intervals (modeled as time.Duration values) with a bound on the number of retry attempts. After the max number of retry attempts has been reached, the dial policy will respond with an error on any further requests for the next retry interval.

### Periodic dial policy

The periodic dial policy generates a bounded number of retry intervals using a fixed period. 

Example usage:

```go
package main

import (
	"github.com/achilleasa/service-adapters"
	"github.com/achilleasa/service-adapters/service/redis"
	"github.com/achilleasa/service-adapters/dial"
	"time"
)

func setup() *redis.Redis {
	// Retry every 200ms up to a total of 10 attempts
	dialPolicy := dial.Periodic(10, time.Millisecond * 200)

	redisSrv, err := redis.New(
		adapters.DialPolicy(dialPolicy),
	)
	if err != nil {
		panic(err)
	}

	return redisSrv
}
```

### Exponential back-off dial policy

The exponential back-off dial policy generates a random retry interval in the range [0, 2<sup>cur. attempt</sup>) with a bound on the total number of attempts. This policy is recommended when a large number of service adaptor instances are running to
prevent connection hammering. The maximum number of attempts that may be specified is 32 (higher numbers will be capped to 32). 

Example usage:

```go
package main

import (
	"github.com/achilleasa/service-adapters"
	"github.com/achilleasa/service-adapters/service/redis"
	"github.com/achilleasa/service-adapters/dial"
	"time"
)

func setup() *redis.Redis {
	// Generate a retry interval in ms between [0, 2^attempt) 
	// up to a total of 10 attempts
	dialPolicy := dial.ExpBackoff(10, time.Millisecond)

	redisSrv, err := redis.New(
		adapters.DialPolicy(dialPolicy),
	)
	if err != nil {
		panic(err)
	}

	return redisSrv
}
```

### Implementing a custom dial policy

To create a custom dial policy you need to implement the [Policy](https://github.com/achilleasa/service-adapters/blob/master/dial/policy.go#L18) interface. You can then pass an instance of the custom dial policy either via the `DialPolicy` service option during service instanciation or via the `SetDialPolicy` method on the instanciated service object.

# Getting started: redis

The redis service adaptor wraps the [redigo](http://github.com/garyburd/redigo/redis) driver. Since the driver is not
thread-safe, the adaptor uses a connection pool. Whenever you need a connection inside a go-routine you can fetch
one from the pool and **close** it when you are done with it.

## Configuration settings

The following configuration settings are supported:

| Setting name | Description           | Default value   |
|--------------|-----------------------|-----------------|
| endpoint     | Redis server endpoint | `localhost:6379`
| password     | The password to use   | `""` (no password)
| db           | The db index to use   | `0`
| connTimeout  | The connection timeout in seconds | `1` second

The default values will be used if no settings are specified.

## Example

```go
package main

import "github.com/achilleasa/service-adapters/service/redis"

func demo() {

	// Use the default periodic dial policy with 1 attempt
	redisSrv, err := redis.New()
	if err != nil {
		panic(err)
	}

	err = redisSrv.Dial()
	if err != nil {
		panic(err)
	}
	defer redisSrv.Close()

	// Get a connection from the pool.
	conn, err := redisSrv.GetConnection()
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// do something with conn here
}
```


# Getting started: rabbitmq

The rabbitmq service adaptor wraps the [go client](https://github.com/streadway/amqp) for AMQP 0.9.1.

## Configuration settings

The following configuration settings are supported:

| Setting name | Description           | Default value   |
|--------------|-----------------------|-----------------|
| endpoint     | rabbit server endpoint including auth credentials | `amqp://guest:guest@localhost:5672/`


The default values will be used if no settings are specified.

## Example

```go
package main

import "github.com/achilleasa/service-adapters/service/amqp"

func demo() {

	// Use the default periodic dial policy with 1 attempt
	rabbitSrv, err := amqp.New()
	if err != nil {
		panic(err)
	}

	err = rabbitSrv.Dial()
	if err != nil {
		panic(err)
	}
	defer rabbitSrv.Close()

	// Allocate amqp channel and do something with it
	channel, err := rabbitSrv.NewChannel()
	if err != nil {
		panic(err)
	}
	defer channel.Close()
}
```

# Automatic service configuration

The `Config` service option (and service instance method) provides a mechanism for delegating the actual
service configuration management to an external service. Automatic configuration (and most importantly re-configuration)
is essential when working with a microservice architecture.

The `etcd` sub-package provides
an automatic configuration server option that uses etcd to retrieve the service settings
from a user-defined key and to set up a monitor for value changes. When a value change is detected, the middleware will
parse the updated value into a map and then reconfigure the service with the updated settings.

The current implementation expects the etcd value to contain a list of ```key=value``` entries (you can use any number of whitespace characters to delimit the value tuples).

## Example

Lets assume that you have launched an etcd v2+ instance and it is currently listening at: `http://127.0.0.1:4001`. Our redis
server is also running at localhost on port 6379

To setup the initial redis settings issue the following curl command:

```
curl http://127.0.0.1:4001/v2/keys/config/service/redis \
     -X PUT \
     -d value="endpoint=127.0.0.1:6379 db=0"
```

The following program will create a redis service adaptor, attach the etcd configuration middleware and a
logger for monitoring configuration change events. It will block till `ctrl+c` is pressed.
     
```go
package main

import (
	"fmt"
	"os"
	"os/signal"

	"log"

	"github.com/achilleasa/service-adapters"
	"github.com/achilleasa/service-adapters/etcd"
	"github.com/achilleasa/service-adapters/service/redis"
)

func main() {

	etcdSrv := etcd.New("http://127.0.0.1:4001")

	redisSrv, err := redis.New(
		adapters.Logger(log.New(os.Stderr, "", log.LstdFlags)),
		etcd.Config(etcdSrv, "/config/service/redis"),
	)
	if err != nil {
		panic(err)
	}
	defer redisSrv.Close()

	fmt.Printf("Press ctrl+c to exit")

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt)
	select {
	case <-sigChan:
	}

	fmt.Printf("Shutting down...")
}
```

When the program executes it will pick up the configuration from etcd and output a message like:

`2015/07/12 18:46:00 [REDIS] Configuration changed; new settings:  endpoint=127.0.0.1:6379, password=, db=0, connTimeout=1s`

You can then change the configuration (in this example the db number and connection timeout) with the following curl command:

```
curl http://127.0.0.1:4001/v2/keys/config/service/redis \
     -X PUT \
     -d value="endpoint=127.0.0.1:6379 db=1 connTimeout=2"
```

The middleware will pickup the change and reconfigure the service adaptor. It will also emit the following logger output:

`2015/07/12 18:46:00 [REDIS] Configuration changed; new settings:  endpoint=127.0.0.1:6379, password=, db=1, connTimeout=2s`

# License

service-adapters is distributed under the [MIT license](https://github.com/achilleasa/service-adapters/blob/master/LICENSE).


