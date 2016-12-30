package sous

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
)

type (
	// HTTPClient interacts with a Sous http server
	//   It's designed to handle basic CRUD operations in a safe and restful way
	HTTPClient struct {
		serverURL *url.URL
		http.Client
	}

	// Comparable is a required interface for Update and Delete, which provides
	// the mechanism for comparing the remote resource to the local data
	Comparable interface {
		// EmptyReceiver should return a pointer to an "zero value" for the recieving type.
		// For example:
		//   func (x *X) EmptyReceiver() { return &X{} }
		EmptyReceiver() Comparable

		// VariancesFrom returns a list of differences from another Comparable.
		// If the two structs are equivalent, it should return an empty list.
		// Usually, the first check will be for identical type, and return "types differ"
		VariancesFrom(Comparable) Variances
	}

	// Variances is a list of differences between two structs
	Variances []string
)

// NewClient returns a new HTTPClient for a particular serverURL
func NewClient(serverURL string) (*HTTPClient, error) {
	u, err := url.Parse(serverURL)

	client := &HTTPClient{
		serverURL: u,
	}

	// XXX: This is in response to a mysterious issue surrounding automatic gzip
	// and Etagging The client receives a gzipped response with "--gzip" appended
	// to the original Etag The --gzip isn't stripped by whatever does it,
	// although the body is decompressed on the server side.  This is a hack to
	// address that issue, which should be resolved properly
	client.Client.Transport = &http.Transport{Proxy: http.ProxyFromEnvironment, DisableCompression: true}

	return client, errors.Wrapf(err, "new Sous REST client")
}

// ****

// Retrieve makes a GET request on urlPath, after transforming qParms into ?&=
// style query params. It deserializes the returned JSON into rzBody. Errors
// are returned if anything goes wrong, including a non-Success HTTP result
// (but note that there may be a response anyway.
func (client *HTTPClient) Retrieve(urlPath string, qParms map[string]string, rzBody interface{}) error {
	return errors.Wrapf(func() error {
		url, err := client.buildURL(urlPath, qParms)
		rq, err := client.buildRequest("GET", url, nil, nil, err)
		rz, err := client.sendRequest(rq, err)
		return client.getBody(rz, rzBody, err)
	}(), "Retrieve %s", urlPath)
}

// Create uses the contents of qBody to create a new resource at the server at urlPath/qParms
// It issues a PUT with "If-No-Match: *", so if a resource already exists, it'll return an error
func (client *HTTPClient) Create(urlPath string, qParms map[string]string, qBody interface{}) error {
	return errors.Wrapf(func() error {
		url, err := client.buildURL(urlPath, qParms)
		rq, err := client.buildRequest("PUT", url, noMatchStar(), qBody, err)
		rz, err := client.sendRequest(rq, err)
		return client.getBody(rz, nil, err)
	}(), "Create %s", urlPath)
}

// Update changes the representation of a given resource.
// It compares the known value to from, and rejects if they're different (on
// the grounds that the client is going to clobber a value it doesn't know
// about.) Then it issues a PUT with "If-Match: <etag of from>" so that the
// server can check that we're changing from a known value.
func (client *HTTPClient) Update(urlPath string, qParms map[string]string, from, qBody Comparable) error {
	return errors.Wrapf(func() error {
		url, err := client.buildURL(urlPath, qParms)
		etag, err := client.getBodyEtag(url, from, err)
		rq, err := client.buildRequest("PUT", url, ifMatch(etag), qBody, err)
		rz, err := client.sendRequest(rq, err)
		return client.getBody(rz, nil, err)
	}(), "Update %s", urlPath)
}

// Delete removes a resource from the server, granted that we know the resource that we're removing.
// It functions similarly to Update, but issues DELETE requests
func (client *HTTPClient) Delete(urlPath string, qParms map[string]string, from Comparable) error {
	return errors.Wrapf(func() error {
		url, err := client.buildURL(urlPath, qParms)
		etag, err := client.getBodyEtag(url, from, err)
		rq, err := client.buildRequest("DELETE", url, ifMatch(etag), nil, err)
		rz, err := client.sendRequest(rq, err)
		return client.getBody(rz, nil, err)
	}(), "Delete %s", urlPath)
}

// ***

func noMatchStar() map[string]string {
	return map[string]string{"If-None-Match": "*"}
}

func ifMatch(etag string) map[string]string {
	return map[string]string{"If-Match": etag}
}

// ****

func (client *HTTPClient) buildURL(urlPath string, qParms map[string]string) (urlS string, err error) {
	URL, err := client.serverURL.Parse(urlPath)
	if err != nil {
		return
	}
	if qParms == nil {
		return URL.String(), nil
	}
	qry := url.Values{}
	for k, v := range qParms {
		qry.Set(k, v)
	}
	URL.RawQuery = qry.Encode()
	return client.serverURL.ResolveReference(URL).String(), nil
}

func (client *HTTPClient) getBodyEtag(url string, body Comparable, ierr error) (etag string, err error) {
	if ierr != nil {
		err = ierr
		return
	}
	Log.Debug.Printf("Getting existing resource from %s", url)

	rzBody := body.EmptyReceiver()

	rq, err := client.buildRequest("GET", url, nil, nil, nil)
	rz, err := client.sendRequest(rq, err)
	err = client.getBody(rz, rzBody, err)
	if err != nil {
		return
	}

	differences := rzBody.VariancesFrom(body)
	if len(differences) > 0 {
		return "", errors.Errorf("Remote and local versions of %s resource don't match: %#v", url, differences)
	}
	return rz.Header.Get("Etag"), nil
}

func (client *HTTPClient) buildRequest(method, url string, headers map[string]string, rqBody interface{}, ierr error) (rq *http.Request, err error) {
	if ierr != nil {
		err = ierr
		return
	}

	Log.Debug.Printf("Sending %s %q", method, url)

	var JSON io.Reader

	if rqBody != nil {
		JSON := &bytes.Buffer{}
		enc := json.NewEncoder(JSON)
		enc.Encode(rqBody)
		Log.Debug.Printf("%s", JSON)
	}

	rq, err = http.NewRequest(method, url, JSON)

	if headers != nil {
		for k, v := range headers {
			rq.Header.Add(k, v)
		}
	}

	return rq, err
}

func (client *HTTPClient) sendRequest(rq *http.Request, ierr error) (rz *http.Response, err error) {
	if ierr != nil {
		err = ierr
		return
	}
	rz, err = client.httpRequest(rq)
	Log.Debug.Printf("Received \"%s %s\" -> %d", rq.Method, rq.URL, rz.StatusCode)
	return
}

func (client *HTTPClient) getBody(rz *http.Response, rzBody interface{}, err error) error {
	if err != nil {
		return err
	}
	defer rz.Body.Close()

	if rzBody != nil {
		dec := json.NewDecoder(rz.Body)
		err = dec.Decode(rzBody)
	}

	if rz.StatusCode != 200 {
		return errors.Errorf("%s: %#v", rz.Status, rzBody)
	}
	return err
}

func (client *HTTPClient) httpRequest(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		Log.Vomit.Printf("-> %s %q", req.Method, req.URL)
	} else {
		req.Body = NewReadDebugger(req.Body, func(b []byte, n int, err error) {
			Log.Vomit.Printf("-> %s %q:\n%sSent %d bytes, result: %v", req.Method, req.URL, string(b), n, err)
		})
	}
	rz, err := client.Client.Do(req)
	log.Print(err)
	if rz == nil {
		return rz, err
	}
	if rz.Body == nil {
		Log.Vomit.Printf("<- %s %q %d", req.Method, req.URL, rz.StatusCode)
	} else {
		rz.Body = NewReadDebugger(rz.Body, func(b []byte, n int, err error) {
			Log.Vomit.Printf("<- %s %q %d:\n%sRead %d bytes, result: %v", req.Method, req.URL, rz.StatusCode, string(b), n, err)
		})
	}
	return rz, err
}
