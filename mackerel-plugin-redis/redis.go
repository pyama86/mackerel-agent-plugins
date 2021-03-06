package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fzzy/radix/redis"
	mp "github.com/mackerelio/go-mackerel-plugin"
	"github.com/mackerelio/mackerel-agent/logging"
)

var logger = logging.GetLogger("metrics.plugin.redis")

var graphdef map[string](mp.Graphs) = map[string](mp.Graphs){
	"redis.queries": mp.Graphs{
		Label: "Redis Queries",
		Unit:  "integer",
		Metrics: [](mp.Metrics){
			mp.Metrics{Name: "instantaneous_ops_per_sec", Label: "Queries", Diff: false},
		},
	},
	"redis.connections": mp.Graphs{
		Label: "Redis Connections",
		Unit:  "integer",
		Metrics: [](mp.Metrics){
			mp.Metrics{Name: "total_connections_received", Label: "Connections", Diff: true, Stacked: true},
			mp.Metrics{Name: "rejected_connections", Label: "Rejected Connections", Diff: true, Stacked: true},
		},
	},
	"redis.clients": mp.Graphs{
		Label: "Redis Clients",
		Unit:  "integer",
		Metrics: [](mp.Metrics){
			mp.Metrics{Name: "connected_clients", Label: "Connected Clients", Diff: false, Stacked: true},
			mp.Metrics{Name: "blocked_clients", Label: "Blocked Clients", Diff: false, Stacked: true},
			mp.Metrics{Name: "connected_slaves", Label: "Blocked Clients", Diff: false, Stacked: true},
		},
	},
	"redis.keys": mp.Graphs{
		Label: "Keys",
		Unit:  "integer",
		Metrics: [](mp.Metrics){
			mp.Metrics{Name: "keys", Label: "Keys", Diff: false},
			mp.Metrics{Name: "expired", Label: "Expired Keys", Diff: false},
		},
	},
	"redis.keyspace": mp.Graphs{
		Label: "Keyspace",
		Unit:  "integer",
		Metrics: [](mp.Metrics){
			mp.Metrics{Name: "keyspace_hits", Label: "Keyspace Hits", Diff: true},
			mp.Metrics{Name: "keyspace_misses", Label: "Keyspace Missed", Diff: true},
		},
	},
	"redis.memory": mp.Graphs{
		Label: "Memory",
		Unit:  "integer",
		Metrics: [](mp.Metrics){
			mp.Metrics{Name: "used_memory", Label: "Used Memory", Diff: false},
			mp.Metrics{Name: "used_memory_rss", Label: "Used Memory RSS", Diff: false},
			mp.Metrics{Name: "used_memory_peak", Label: "Used Memory Peak", Diff: false},
			mp.Metrics{Name: "used_memory_lua", Label: "Used Memory Lua engine", Diff: false},
		},
	},
}

type RedisPlugin struct {
	Target   string
	Timeout  int
	Tempfile string
}

func (m RedisPlugin) FetchMetrics() (map[string]float64, error) {
	c, err := redis.DialTimeout("tcp", m.Target, time.Duration(m.Timeout)*time.Second)
	defer c.Close()

	r := c.Cmd("info")
	if r.Err != nil {
		logger.Errorf("Failed to run info command. %s", r.Err)
		return nil, r.Err
	}
	str, err := r.Str()
	if err != nil {
		logger.Errorf("Failed to fetch information. %s", err)
		return nil, err
	}

	stat := make(map[string]float64)

	for _, line := range strings.Split(str, "\r\n") {
		if line == "" {
			continue
		}
		if re, _ := regexp.MatchString("^#", line); re {
			continue
		}

		record := strings.SplitN(line, ":", 2)
		if len(record) < 2 {
			continue
		}
		key, value := record[0], record[1]

		if re, _ := regexp.MatchString("^db", key); re {
			kv := strings.SplitN(value, ",", 3)
			keys, expired := kv[0], kv[1]

			keys_kv := strings.SplitN(keys, "=", 2)
			keys_fv, err := strconv.ParseFloat(keys_kv[1], 64)
			if err != nil {
				logger.Warningf("Failed to parse db keys. %s", err)
			}
			stat["keys"] += keys_fv

			expired_kv := strings.SplitN(expired, "=", 2)
			expired_fv, err := strconv.ParseFloat(expired_kv[1], 64)
			if err != nil {
				logger.Warningf("Failed to parse db expired. %s", err)
			}
			stat["expired"] += expired_fv

			continue
		}

		stat[key], err = strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}

	}

	if _, ok := stat["keys"]; !ok {
		stat["keys"] = 0
	}
	if _, ok := stat["expired"]; !ok {
		stat["expired"] = 0
	}

	return stat, nil
}

func (m RedisPlugin) GraphDefinition() map[string](mp.Graphs) {
	return graphdef
}

func main() {
	optHost := flag.String("host", "localhost", "Hostname")
	optPort := flag.String("port", "6379", "Port")
	optTimeout := flag.Int("timeout", 5, "Timeout")
	optTempfile := flag.String("tempfile", "", "Temp file name")
	flag.Parse()

	var redis RedisPlugin
	redis.Target = fmt.Sprintf("%s:%s", *optHost, *optPort)
	redis.Timeout = *optTimeout
	helper := mp.NewMackerelPlugin(redis)

	if *optTempfile != "" {
		helper.Tempfile = *optTempfile
	} else {
		helper.Tempfile = fmt.Sprintf("/tmp/mackerel-plugin-redis-%s-%s", *optHost, *optPort)
	}

	if os.Getenv("MACKEREL_AGENT_PLUGIN_META") != "" {
		helper.OutputDefinitions()
	} else {
		helper.OutputValues()
	}
}
