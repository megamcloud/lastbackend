//
// Last.Backend LLC CONFIDENTIAL
// __________________
//
// [2014] - [2018] Last.Backend LLC
// All Rights Reserved.
//
// NOTICE:  All information contained herein is, and remains
// the property of Last.Backend LLC and its suppliers,
// if any.  The intellectual and technical concepts contained
// herein are proprietary to Last.Backend LLC
// and its suppliers and may be covered by Russian Federation and Foreign Patents,
// patents in process, and are protected by trade secret or copyright law.
// Dissemination of this information or reproduction of this material
// is strictly forbidden unless prior written permission is obtained
// from Last.Backend LLC.
//

package ipvs

import (
	"github.com/lastbackend/lastbackend/pkg/node/runtime/cpi"
	"context"
	"github.com/lastbackend/lastbackend/pkg/distribution/types"
	"github.com/lastbackend/dynamic/pkg/log"
	"strings"
	"strconv"
	"github.com/lastbackend/dynamic/pkg/distribution/errors"
)

const logIPVSPrefix = "cpi:ivps:proxy:>"

// Proxy balancer
type Proxy struct {
	cpi cpi.CPI
	// IVPS cmd path
	ipvs *IPVS
}

func (i *Proxy) Info(ctx context.Context) (map[string]*types.EndpointStatus, error) {
	el := make(map[string]*types.EndpointStatus)

	svcs, err := i.ipvs.GetServices(ctx)
	if err != nil {
		log.Errorf("%s info error: %s", logIPVSPrefix, err.Error())
		return nil, err
	}

	for _, svc := range svcs {

		// check if endpoint exists
		if _, ok := el[svc.Host]; !ok {
			el[svc.Host] = new(types.EndpointStatus)
			el[svc.Host].Upstreams = make(map[int]map[string]*types.EndpointUpstream)
		}

		for _, bknd := range svc.Backends {

			if _, ok := el[svc.Host].Upstreams[svc.Port]; !ok {
				el[svc.Host].Upstreams[svc.Port] = make(map[string]*types.EndpointUpstream)
			}

			el[svc.Host].Upstreams[svc.Port][bknd.Host] = &types.EndpointUpstream{
				Host: bknd.Host,
				Port: bknd.Port,
			}

		}
	}

	return el, nil
}

// Create new proxy rules
func (i *Proxy) Create(ctx context.Context, spec *types.EndpointSpec) (*types.EndpointStatus, error) {

	var (
		err  error
		svcs = make([]*Service, 0)
		status = new(types.EndpointStatus)
	)

	status.Upstreams = make(map[int]map[string]*types.EndpointUpstream)

	defer func() {
		if err != nil {
			for _, svc := range svcs {
				i.ipvs.DelService(ctx, svc)
			}
		}
	}()

	for ext, it := range spec.PortMap {

		var (
			port  int
			proto string
			err   error
		)

		// Setup port and proxy type for IPVS service
		pm := strings.Split(ext, "/")
		switch len(pm) {
		case 0:
			continue
			break
		case 1:
			port, err = strconv.Atoi(pm[0])
			if err != nil {
				continue
			}
			proto = "*"
			break
		case 2:
			port, err = strconv.Atoi(pm[0])
			if err != nil {
				continue
			}
			proto = strings.ToLower(pm[1])
			break
		default:
			err = errors.New("Invalid port map declaration")
			status.State = types.StateError
			status.Message = err.Error()
			return status, err
		}

		status.Upstreams[port] = make(map[string]*types.EndpointUpstream)

		svc := Service{
			Host: spec.IP,
			Port: port,
		}

		for _, ups := range spec.Upstreams {
			svc.Backends = append(svc.Backends, Backend{
				Host: ups.Host,
				Port: it,
			})

			status.Upstreams[port][ups.Host] = ups
		}

		switch proto {
		case "tcp":
			svc.Type = proxyTCPProto
			if err := i.ipvs.AddService(ctx, &svc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}
			svcs = append(svcs, &svc)
			status.State = types.StateCreated
			status.Message = ""
			break
		case "udp":
			svc.Type = proxyUDPProto
			if err := i.ipvs.AddService(ctx, &svc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}
			svcs = append(svcs, &svc)
			status.State = types.StateCreated
			status.Message = ""
			break
		case "*":
			svcc := svc
			svc.Type = proxyTCPProto
			svcc.Type = proxyUDPProto

			if err := i.ipvs.AddService(ctx, &svc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}
			svcs = append(svcs, &svc)

			if err := i.ipvs.AddService(ctx, &svcc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}
			svcs = append(svcs, &svcc)

			status.State = types.StateCreated
			status.Message = ""
			break
		}
	}

	return status, nil
}

func (i *Proxy) Destroy(ctx context.Context, spec *types.EndpointSpec) (*types.EndpointStatus, error) {

	var (
		err  error
		svcs = make([]*Service, 0)
		status = new(types.EndpointStatus)
	)

	defer func() {
		if err != nil {
			for _, svc := range svcs {
				i.ipvs.DelService(ctx, svc)
			}
		}
	}()

	for ext := range spec.PortMap {

		var (
			port  int
			proto string
			err   error
		)

		// Setup port and proxy type for IPVS service
		pm := strings.Split(ext, "/")
		switch len(pm) {
		case 0:
			continue
			break
		case 1:
			port, err = strconv.Atoi(pm[0])
			if err != nil {
				continue
			}
			proto = "*"
			break
		case 2:
			port, err = strconv.Atoi(pm[0])
			if err != nil {
				continue
			}
			proto = strings.ToLower(pm[1])
			break
		default:
			err = errors.New("Invalid port map declaration")
			status.State = types.StateError
			status.Message = err.Error()
			return status, err
		}

		svc := Service{
			Host: spec.IP,
			Port: port,
		}

		switch proto {
		case "tcp":
			svc.Type = proxyTCPProto
			if err := i.ipvs.DelService(ctx, &svc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}
			status.State = types.StateDestroyed
			status.Message = ""
			break
		case "udp":
			svc.Type = proxyUDPProto
			if err := i.ipvs.DelService(ctx, &svc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}
			status.State = types.StateDestroyed
			status.Message = ""
			break
		case "*":
			svcc := svc
			svc.Type = proxyTCPProto
			svcc.Type = proxyUDPProto

			if err := i.ipvs.DelService(ctx, &svc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}

			if err := i.ipvs.DelService(ctx, &svcc); err != nil {
				status.State = types.StateError
				status.Message = err.Error()
				return status, err
			}
			status.State = types.StateDestroyed
			status.Message = ""

			break
		}
	}

	return status, nil
}

func (p *Proxy) Replace(ctx context.Context, spec *types.EndpointSpec) (*types.EndpointStatus, error) {


}

func New() (*Proxy, error) {
	prx := new(Proxy)
	// TODO: Check ipvs proxy mode is available on host
	return prx, nil
}
