package toxiproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Shopify/toxiproxy/toxics"
	"github.com/sirupsen/logrus"
)

// ProxyCollection is a collection of proxies. It's the interface for anything
// to add and remove proxies from the toxiproxy instance. It's responsibilty is
// to maintain the integrity of the proxy set, by guarding for things such as
// duplicate names.
type ProxyCollection struct {
	sync.RWMutex

	proxies map[string]*Proxy
}

func NewProxyCollection() *ProxyCollection {
	return &ProxyCollection{
		proxies: make(map[string]*Proxy),
	}
}

func (collection *ProxyCollection) Add(proxy *Proxy, start bool) error {
	collection.Lock()
	defer collection.Unlock()

	if _, exists := collection.proxies[proxy.Name]; exists {
		return ErrProxyAlreadyExists
	}

	if start {
		err := proxy.Start()
		if err != nil {
			return err
		}
	}

	collection.proxies[proxy.Name] = proxy

	return nil
}

func (collection *ProxyCollection) AddOrReplace(proxy *Proxy, start bool) error {
	collection.Lock()
	defer collection.Unlock()

	if existing, exists := collection.proxies[proxy.Name]; exists {
		if existing.Listen == proxy.Listen && existing.Upstream == proxy.Upstream {
			return nil
		}
		existing.Stop()
	}

	if start {
		err := proxy.Start()
		if err != nil {
			return err
		}
	}

	collection.proxies[proxy.Name] = proxy

	return nil
}

func (collection *ProxyCollection) PopulateJson(data io.Reader) ([]*Proxy, error) {
	input := []struct {
		Proxy
		Enabled *bool `json:"enabled"` // Overrides Proxy field to make field nullable
	}{}

	err := json.NewDecoder(data).Decode(&input)
	if err != nil {
		return nil, joinError(err, ErrBadRequestBody)
	}

	// Check for valid input before creating any proxies
	t := true
	for i, p := range input {
		if len(p.Name) < 1 {
			return nil, joinError(fmt.Errorf("name at proxy %d", i+1), ErrMissingField)
		}
		if len(p.Upstream) < 1 {
			return nil, joinError(fmt.Errorf("upstream at proxy %d", i+1), ErrMissingField)
		}
		if p.Enabled == nil {
			input[i].Enabled = &t
		}
	}

	proxies := make([]*Proxy, 0, len(input))

	for _, p := range input {
		proxy := NewProxy()
		proxy.Name = p.Name
		proxy.Listen = p.Listen
		proxy.Upstream = p.Upstream

		err = collection.AddOrReplace(proxy, *p.Enabled)
		if err != nil {
			break
		}

		proxies = append(proxies, proxy)
	}
	return proxies, err
}

func (collection *ProxyCollection) PopulateToxicsJson(data io.Reader) ([]*toxics.ToxicWrapper, error) {
	toxicConfig := []struct {
		Name   string             `json:"name"`
		Toxics []toxics.ToxicJSON `json:"toxics"`
	}{}

	err := json.NewDecoder(data).Decode(&toxicConfig)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode toxicConfig: %v", err)
	}

	toxicList := []*toxics.ToxicWrapper{}

	for _, p := range toxicConfig {
		proxy, err := collection.Get(p.Name)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"config":    data,
				"proxyName": p.Name,
				"error":     err,
			}).Error("Failed to populate proxies from file")
			break
		}

		for _, t := range p.Toxics {
			data, err := json.Marshal(t)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"config": data,
					"error":  err,
				}).Error("Failed marshal json")
				break
			}

			toxic, err := proxy.Toxics.AddToxicJson(strings.NewReader(string(data)))
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"config": data,
					"error":  err,
				}).Error("Failed to populate toxics")
				break
			}

			toxicList = append(toxicList, toxic)

		}

	}

	return toxicList, nil
}

func (collection *ProxyCollection) Proxies() map[string]*Proxy {
	collection.RLock()
	defer collection.RUnlock()

	// Copy the map since using the existing one isn't thread-safe
	proxies := make(map[string]*Proxy, len(collection.proxies))
	for k, v := range collection.proxies {
		proxies[k] = v
	}
	return proxies
}

func (collection *ProxyCollection) Get(name string) (*Proxy, error) {
	collection.RLock()
	defer collection.RUnlock()

	return collection.getByName(name)
}

func (collection *ProxyCollection) Remove(name string) error {
	collection.Lock()
	defer collection.Unlock()

	proxy, err := collection.getByName(name)
	if err != nil {
		return err
	}
	proxy.Stop()

	delete(collection.proxies, proxy.Name)
	return nil
}

func (collection *ProxyCollection) Clear() error {
	collection.Lock()
	defer collection.Unlock()

	for _, proxy := range collection.proxies {
		proxy.Stop()

		delete(collection.proxies, proxy.Name)
	}

	return nil
}

// getByName returns a proxy by its name. Its used from #remove and #get.
// It assumes the lock has already been acquired.
func (collection *ProxyCollection) getByName(name string) (*Proxy, error) {
	proxy, exists := collection.proxies[name]
	if !exists {
		return nil, ErrProxyNotFound
	}
	return proxy, nil
}
