// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gorilla/securecookie"

	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/mux"
	"github.com/ssb-ngi-pointer/go-ssb-room/internal/repo"
	"go.mindeco.de/logging"
)

//go:generate go run -tags=dev embedded_generate.go

func TemplateFuncs(m *mux.Router) template.FuncMap {
	return template.FuncMap{
		"urlTo": NewURLTo(m),
		"inc":   func(i int) int { return i + 1 },
	}
}

func NewURLTo(appRouter *mux.Router) func(string, ...interface{}) *url.URL {
	l := logging.Logger("helper.URLTo") // TOOD: inject in a scoped way
	return func(routeName string, ps ...interface{}) *url.URL {
		route := appRouter.Get(routeName)
		if route == nil {
			level.Warn(l).Log("msg", "no such route", "route", routeName, "params", ps)
			return &url.URL{}
		}

		var params []string
		for _, p := range ps {
			switch v := p.(type) {
			case string:
				params = append(params, v)
			case int:
				params = append(params, strconv.Itoa(v))
			case int64:
				params = append(params, strconv.FormatInt(v, 10))

			default:
				level.Error(l).Log("msg", "invalid param type", "param", p, "route", routeName)
				logging.CheckFatal(errors.New("invalid param"))
			}
		}

		u, err := route.URLPath(params...)
		if err != nil {
			level.Error(l).Log("msg", "failed to create URL",
				"route", routeName,
				"params", params,
				"error", err)
			return &url.URL{}
		}
		return u
	}
}

// LoadOrCreateCookieSecrets either parses the bytes from $repo/web/cookie-secret or creates a new file with suitable keys in it
func LoadOrCreateCookieSecrets(repo repo.Interface) ([]securecookie.Codec, error) {
	secretPath := repo.GetPath("web", "cookie-secret")
	err := os.MkdirAll(filepath.Dir(secretPath), 0700)
	if err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("failed to create folder for cookie secret: %w", err)
	}

	// load the existing data
	secrets, err := ioutil.ReadFile(secretPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load cookie secrets: %w", err)
		}

		// create new keys, save them and return the codec
		hashKey := securecookie.GenerateRandomKey(32)
		blockKey := securecookie.GenerateRandomKey(32)

		data := append(hashKey, blockKey...)
		err = ioutil.WriteFile(secretPath, data, 0600)
		if err != nil {
			return nil, err
		}
		sc := securecookie.CodecsFromPairs(hashKey, blockKey)
		return sc, nil
	}

	// secrets should contain multiple of 64byte (to enable key rotation as supported by gorilla)
	if n := len(secrets); n%64 != 0 {
		return nil, fmt.Errorf("expected multiple of 64bytes in cookie secret file but got: %d", n)
	}

	// range over the secrets []byte in chunks of 64 bytes
	// and slice it into 32byte pairs
	var pairs [][]byte

	// the increment/next part (which usually is i++)
	// is the multiple comma assigment (a,b = b+1,a-1)
	// so chunk is the next 64 bytes and then it slices of the first 64 bytes of secrets for the next iteration
	for chunk := secrets[:64]; len(secrets) >= 64; chunk, secrets = secrets[:64], secrets[64:] {
		pairs = append(pairs,
			chunk[0:32],  // hash key
			chunk[32:64], // block key
		)
	}

	sc := securecookie.CodecsFromPairs(pairs...)
	return sc, nil
}