// Package firebase impleements a RESTful client for Firebase.
package firebase

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Api is the interface for interacting with Firebase.
// Consumers of this package can mock this interface for testing purposes.
type Api interface {
	Call(method, path, auth string, body []byte, params map[string]string) ([]byte, error)
}

// F is the Firebase client.
type F struct {
	// Url is the client's base URL used for all calls.
	Url string

	// Auth is authentication token used when making calls.
	// The token is optional and can also be overwritten on an individual
	// call basis via params.
	Auth string

	// api is the underlying client used to make calls.
	api Api

	// value is the value of the object at the current Url
	value interface{}
}

// struct is the internal implementation of the Firebase API client.
type client struct{}

// suffix is the Firebase suffix for invoking their API via HTTP
const suffix = ".json"

// httpClient is the HTTP client used to make calls to Firebase
var httpClient = new(http.Client)

// Init initializes the Firebase client with a given root url and optional auth token.
// The initialization can also pass a mock api for testing purposes.
func (f *F) Init(root, auth string, api Api) {
	if api == nil {
		api = new(client)
	}

	f.api = api
	f.Url = root
	f.Auth = auth
}

// Value returns the value of of the current Url.
func (f *F) Value() interface{} {
	// if we have not yet performed a look-up, do it so a value is returned
	if f.value == nil {
		var v interface{}
		f = f.Child("", nil, v)
	}

	if f == nil {
		return nil
	}

	return f.value
}

// Child returns a populated pointer for a given path.
// If the path cannot be found, a null pointer is returned.
func (f *F) Child(path string, params map[string]string, v interface{}) *F {
	u := f.Url + "/" + path

	res, err := f.api.Call("GET", u, f.Auth, nil, params)
	if err != nil {
		return nil
	}

	err = json.Unmarshal(res, &v)
	if err != nil {
		log.Printf("%v\n", err)
		return nil
	}

	ret := &F{
		api:   f.api,
		Auth:  f.Auth,
		Url:   u,
		value: v}

	return ret
}

// Push creates a new value under the current root url.
// A populated pointer with that value is also returned.
func (f *F) Push(value interface{}, params map[string]string) (*F, error) {
	body, err := json.Marshal(value)
	if err != nil {
		log.Printf("%v\n", err)
		return nil, err
	}

	res, err := f.api.Call("POST", f.Url, f.Auth, body, params)
	if err != nil {
		return nil, err
	}

	var r map[string]string

	err = json.Unmarshal(res, &r)
	if err != nil {
		log.Printf("%v\n", err)
		return nil, err
	}

	ret := &F{
		api:   f.api,
		Auth:  f.Auth,
		Url:   f.Url + "/" + r["name"],
		value: value}

	return ret, nil
}

// Set overwrites the value at the specified path and returns populated pointer
// for the updated path.
func (f *F) Set(path string, value interface{}, params map[string]string) (*F, error) {
	u := f.Url + "/" + path

	body, err := json.Marshal(value)
	if err != nil {
		log.Printf("%v\n", err)
		return nil, err
	}

	res, err := f.api.Call("PUT", u, f.Auth, body, params)

	if err != nil {
		return nil, err
	}

	ret := &F{
		api:  f.api,
		Auth: f.Auth,
		Url:  u}

	if len(res) > 0 {
		var r interface{}

		err = json.Unmarshal(res, &r)
		if err != nil {
			log.Printf("%v\n", err)
			return nil, err
		}

		ret.value = r
	}

	return ret, nil
}

// Update performs a partial update with the given value at the specified path.
func (f *F) Update(path string, value interface{}, params map[string]string) error {
	body, err := json.Marshal(value)
	if err != nil {
		log.Printf("%v\n", err)
		return err
	}

	_, err = f.api.Call("PATCH", f.Url+"/"+path, f.Auth, body, params)

	// if we've just updated the root node, clear the value so it gets looked up
	// again and populated correctly since we just applied a diffgram
	if len(path) == 0 {
		f.value = nil
	}

	return err
}

// Remove deletes the data at the given path.
func (f *F) Remove(path string, params map[string]string) error {
	_, err := f.api.Call("DELETE", f.Url+"/"+path, f.Auth, nil, params)

	return err
}

// Call invokes the appropriate HTTP method on a given Firebase URL.
func (c *client) Call(method, path, auth string, body []byte, params map[string]string) ([]byte, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	path += suffix
	qs := url.Values{}

	// if the client has an auth, set it as a query string.
	// the caller can also override this on a per-call basis
	// which will happen via params below
	if len(auth) > 0 {
		qs.Set("auth", auth)
	}

	for k, v := range params {
		qs.Set(k, v)
	}

	if len(qs) > 0 {
		path += "?" + qs.Encode()
	}

	req, err := http.NewRequest(method, path, bytes.NewReader(body))
	if err != nil {
		log.Printf("Cannot create Firebase request: %v\n", err)
		return nil, err
	}

	req.Close = true
	log.Printf("Calling %v %q\n", method, path)

	res, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Request to Firebase failed: %v\n", err)
		return nil, err
	}
	defer res.Body.Close()

	ret, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Cannot parse Firebase response: %v\n", err)
		return nil, err
	}

	if res.StatusCode >= 400 {
		err = errors.New(string(ret))
		log.Printf("Error encountered from Firebase: %v\n", err)
		return nil, err
	}

	return ret, nil
}
