package portallocator

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
)

const (
	DefaultPortRangeStart = 49153
	DefaultPortRangeEnd   = 65535
)

var (
	beginPortRange = DefaultPortRangeStart
	endPortRange   = DefaultPortRangeEnd
)

type portMap struct {
	p    map[int]struct{}
	last int
}

func newPortMap() *portMap {
	return &portMap{
		p:    map[int]struct{}{},
		last: endPortRange,
	}
}

type protoMap map[string]*portMap

func newProtoMap() protoMap {
	return protoMap{
		"tcp": newPortMap(),
		"udp": newPortMap(),
	}
}

type ipMapping map[string]protoMap

var (
	ErrAllPortsAllocated = errors.New("all ports are allocated")
	ErrUnknownProtocol   = errors.New("unknown protocol")
)

var (
	mutex sync.Mutex

	defaultIP = net.ParseIP("0.0.0.0")
	globalMap = ipMapping{}
)

type ErrPortAlreadyAllocated struct {
	ip   string
	port int
}

func NewErrPortAlreadyAllocated(ip string, port int) ErrPortAlreadyAllocated {
	return ErrPortAlreadyAllocated{
		ip:   ip,
		port: port,
	}
}

func init() {
	const param = "/proc/sys/net/ipv4/ip_local_port_range"

	if line, err := ioutil.ReadFile(param); err != nil {
		log.Errorf("Failed to read %s kernel parameter: %s", param, err.Error())
	} else {
		var start, end int
		if n, err := fmt.Fscanf(strings.NewReader(string(line)), "%d\t%d", &start, &end); n != 2 || err != nil {
			if err == nil {
				err = fmt.Errorf("unexpected count of parsed numbers (%d)", n)
			}
			log.Errorf("Failed to parse port range from %s: %v", param, err)
		} else {
			beginPortRange = start
			endPortRange = end
		}
	}
}

func GetPortRange() (int, int) {
	return beginPortRange, endPortRange
}

func (e ErrPortAlreadyAllocated) IP() string {
	return e.ip
}

func (e ErrPortAlreadyAllocated) Port() int {
	return e.port
}

func (e ErrPortAlreadyAllocated) IPPort() string {
	return fmt.Sprintf("%s:%d", e.ip, e.port)
}

func (e ErrPortAlreadyAllocated) Error() string {
	return fmt.Sprintf("Bind for %s:%d failed: port is already allocated", e.ip, e.port)
}

// RequestPort requests new port from global ports pool for specified ip and proto.
// If port is 0 it returns first free port. Otherwise it cheks port availability
// in pool and return that port or error if port is already busy.
func RequestPort(ip net.IP, proto string, port int) (int, error) {
	mutex.Lock()
	defer mutex.Unlock()

	if proto != "tcp" && proto != "udp" {
		return 0, ErrUnknownProtocol
	}

	if ip == nil {
		ip = defaultIP
	}
	ipstr := ip.String()
	protomap, ok := globalMap[ipstr]
	if !ok {
		protomap = newProtoMap()
		globalMap[ipstr] = protomap
	}
	mapping := protomap[proto]
	if port > 0 {
		if _, ok := mapping.p[port]; !ok {
			mapping.p[port] = struct{}{}
			return port, nil
		}
		return 0, NewErrPortAlreadyAllocated(ipstr, port)
	}

	port, err := mapping.findPort()
	if err != nil {
		return 0, err
	}
	return port, nil
}

// ReleasePort releases port from global ports pool for specified ip and proto.
func ReleasePort(ip net.IP, proto string, port int) error {
	mutex.Lock()
	defer mutex.Unlock()

	if ip == nil {
		ip = defaultIP
	}
	protomap, ok := globalMap[ip.String()]
	if !ok {
		return nil
	}
	delete(protomap[proto].p, port)
	return nil
}

// ReleaseAll releases all ports for all ips.
func ReleaseAll() error {
	mutex.Lock()
	globalMap = ipMapping{}
	mutex.Unlock()
	return nil
}

func (pm *portMap) findPort() (int, error) {
	port := pm.last
	start, end := GetPortRange()
	for i := 0; i <= end-start; i++ {
		port++
		if port > end {
			port = start
		}

		if _, ok := pm.p[port]; !ok {
			pm.p[port] = struct{}{}
			pm.last = port
			return port, nil
		}
	}
	return 0, ErrAllPortsAllocated
}
