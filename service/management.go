package service

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/IceWhaleTech/CasaOS-Gateway/common"
)

const RoutesFile = "routes.json"

type Management struct {
	pathTargetMap       map[string]string
	pathReverseProxyMap map[string]*httputil.ReverseProxy
	state               *State
}

func NewManagementService(state *State) *Management {
	routesFilepath := filepath.Join(state.GetRuntimePath(), RoutesFile)

	// try to load routes from routes.json
	pathTargetMap, err := loadPathTargetMapFrom(routesFilepath)
	if err != nil {
		log.Println(err)
		pathTargetMap = make(map[string]string)
	}

	pathReverseProxyMap := make(map[string]*httputil.ReverseProxy)

	for path, target := range pathTargetMap {
		targetURL, err := url.Parse(target)
		if err != nil {
			log.Println(err)
			continue
		}
		pathReverseProxyMap[path] = httputil.NewSingleHostReverseProxy(targetURL)
	}

	return &Management{
		pathTargetMap:       pathTargetMap,
		pathReverseProxyMap: pathReverseProxyMap,
		state:               state,
	}
}

func (g *Management) CreateRoute(route *common.Route) error {
	url, err := url.Parse(route.Target)
	if err != nil {
		return err
	}

	g.pathTargetMap[route.Path] = route.Target
	g.pathReverseProxyMap[route.Path] = httputil.NewSingleHostReverseProxy(url)

	routesFilePath := filepath.Join(g.state.GetRuntimePath(), RoutesFile)

	err = savePathTargetMapTo(routesFilePath, g.pathTargetMap)
	if err != nil {
		return err
	}

	return nil
}

func (g *Management) GetRoutes() []*common.Route {
	routes := make([]*common.Route, 0)

	for path, target := range g.pathTargetMap {
		routes = append(routes, &common.Route{
			Path:   path,
			Target: target,
		})
	}

	return routes
}

func (g *Management) GetProxy(path string) *httputil.ReverseProxy {
	for p, proxy := range g.pathReverseProxyMap {
		if strings.HasPrefix(path, p) {
			return proxy
		}
	}
	return nil
}

func (g *Management) GetGatewayPort() string {
	return g.state.GetGatewayPort()
}

func (g *Management) SetGatewayPort(port string) error {
	if err := g.state.SetGatewayPort(port); err != nil {
		return err
	}

	return nil
}

func loadPathTargetMapFrom(routesFilepath string) (map[string]string, error) {
	content, err := ioutil.ReadFile(routesFilepath)
	if err != nil {
		return nil, err
	}

	pathTargetMap := make(map[string]string)
	err = json.Unmarshal(content, &pathTargetMap)
	if err != nil {
		return nil, err
	}

	return pathTargetMap, nil
}

func savePathTargetMapTo(routesFilepath string, pathTargetMap map[string]string) error {
	content, err := json.Marshal(pathTargetMap)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(routesFilepath, content, 0o600)
}
