package couchdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/flimzy/diff"
	"github.com/flimzy/kivik"
	"github.com/flimzy/kivik/driver"
	"github.com/flimzy/testy"
)

func TestAllDocs(t *testing.T) {
	db := newTestDB(nil, errors.New("test error"))
	_, err := db.AllDocs(context.Background(), nil)
	testy.Error(t, "Get http://example.com/testdb/_all_docs: test error", err)
}

func TestQuery(t *testing.T) {
	db := newTestDB(nil, errors.New("test error"))
	_, err := db.Query(context.Background(), "ddoc", "view", nil)
	testy.Error(t, "Get http://example.com/testdb/_design/ddoc/_view/view: test error", err)
}

func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		db       *db
		id       string
		options  map[string]interface{}
		expected string
		err      string
	}{
		{
			name: "missing doc ID",
			err:  "kivik: docID required",
		},
		{
			name:    "invalid options",
			id:      "foo",
			options: map[string]interface{}{"foo": make(chan int)},
			err:     "cannot convert type chan int to []string",
		},
		{
			name: "network failure",
			id:   "foo",
			db:   newTestDB(nil, errors.New("net error")),
			err:  "Get http://example.com/testdb/foo: net error",
		},
		{
			name: "error response",
			id:   "foo",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusBadRequest,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}, nil),
			err: "Bad Request",
		},
		{
			name: "status OK",
			id:   "foo",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader("some response")),
			}, nil),
			expected: "some response",
		},
		{
			name: "body read failure",
			id:   "foo",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Body:       errorReadCloser{errors.New("read error")},
			}, nil),
			err: "read error",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.db.Get(context.Background(), test.id, test.options)
			testy.Error(t, test.err, err)
			if string(result) != test.expected {
				t.Errorf("Unexpected result: %s", string(result))
			}
		})
	}
}

func TestCreateDoc(t *testing.T) {
	tests := []struct {
		name         string
		db           *db
		doc          interface{}
		id, rev, err string
	}{
		{
			name: "network error",
			db:   newTestDB(nil, errors.New("foo error")),
			err:  "Post http://example.com/testdb: foo error",
		},
		{
			name: "invalid doc",
			doc:  make(chan int),
			db:   newTestDB(nil, errors.New("")),
			err:  "json: unsupported type: chan int",
		},
		{
			name: "error response",
			doc:  map[string]interface{}{"foo": "bar"},
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusBadRequest,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}, nil),
			err: "Bad Request",
		},
		{
			name: "invalid JSON response",
			doc:  map[string]interface{}{"foo": "bar"},
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader("invalid json")),
			}, nil),
			err: "invalid character 'i' looking for beginning of value",
		},
		{
			name: "success, 1.6.1",
			doc:  map[string]interface{}{"foo": "bar"},
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Header: map[string][]string{
					"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
					"Location":       {"http://localhost:5984/foo/43734cf3ce6d5a37050c050bb600006b"},
					"ETag":           {`"1-4c6114c65e295552ab1019e2b046b10e"`},
					"Date":           {"Wed, 25 Oct 2017 10:38:38 GMT"},
					"Content-Type":   {"text/plain; charset=utf-8"},
					"Content-Length": {"95"},
					"Cache-Control":  {"must-revalidate"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"ok":true,"id":"43734cf3ce6d5a37050c050bb600006b","rev":"1-4c6114c65e295552ab1019e2b046b10e"}
`)),
			}, nil),
			id:  "43734cf3ce6d5a37050c050bb600006b",
			rev: "1-4c6114c65e295552ab1019e2b046b10e",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, rev, err := test.db.CreateDoc(context.Background(), test.doc)
			testy.Error(t, test.err, err)
			if test.id != id || test.rev != rev {
				t.Errorf("Unexpected results: ID=%s rev=%s", id, rev)
			}
		})
	}
}

func TestStats(t *testing.T) {
	tests := []struct {
		name     string
		db       *db
		expected *driver.DBStats
		err      string
	}{
		{
			name: "network error",
			db:   newTestDB(nil, errors.New("net error")),
			err:  "Get http://example.com/testdb: net error",
		},
		{
			name: "1.6.1",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Header: http.Header{
					"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
					"Date":           {"Thu, 26 Oct 2017 12:58:14 GMT"},
					"Content-Type":   {"text/plain; charset=utf-8"},
					"Content-Length": {"235"},
					"Cache-Control":  {"must-revalidate"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"db_name":"_users","doc_count":3,"doc_del_count":14,"update_seq":31,"purge_seq":0,"compact_running":false,"disk_size":127080,"data_size":6028,"instance_start_time":"1509022681259533","disk_format_version":6,"committed_update_seq":31}`)),
			}, nil),
			expected: &driver.DBStats{
				Name:         "_users",
				DocCount:     3,
				DeletedCount: 14,
				UpdateSeq:    "31",
				DiskSize:     127080,
				ActiveSize:   6028,
			},
		},
		{
			name: "2.0.0",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Header: http.Header{
					"Server":              {"CouchDB/2.0.0 (Erlang OTP/17)"},
					"Date":                {"Thu, 26 Oct 2017 13:01:13 GMT"},
					"Content-Type":        {"application/json"},
					"Content-Length":      {"429"},
					"Cache-Control":       {"must-revalidate"},
					"X-Couch-Request-ID":  {"2486f27546"},
					"X-CouchDB-Body-Time": {"0"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"db_name":"_users","update_seq":"13-g1AAAAEzeJzLYWBg4MhgTmHgzcvPy09JdcjLz8gvLskBCjMlMiTJ____PyuRAYeCJAUgmWQPVsOCS40DSE08WA0rLjUJIDX1eO3KYwGSDA1ACqhsPiF1CyDq9mclMuFVdwCi7j4hdQ8g6kDuywIAkRBjAw","sizes":{"file":87323,"external":2495,"active":6082},"purge_seq":0,"other":{"data_size":2495},"doc_del_count":6,"doc_count":1,"disk_size":87323,"disk_format_version":6,"data_size":6082,"compact_running":false,"instance_start_time":"0"}`)),
			}, nil),
			expected: &driver.DBStats{
				Name:         "_users",
				DocCount:     1,
				DeletedCount: 6,
				UpdateSeq:    "13-g1AAAAEzeJzLYWBg4MhgTmHgzcvPy09JdcjLz8gvLskBCjMlMiTJ____PyuRAYeCJAUgmWQPVsOCS40DSE08WA0rLjUJIDX1eO3KYwGSDA1ACqhsPiF1CyDq9mclMuFVdwCi7j4hdQ8g6kDuywIAkRBjAw",
				DiskSize:     87323,
				ActiveSize:   6082,
				ExternalSize: 2495,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.db.Stats(context.Background())
			testy.Error(t, test.err, err)
			if d := diff.Interface(test.expected, result); d != nil {
				t.Error(d)
			}
		})
	}
}

func TestOptionsToParams(t *testing.T) {
	type otpTest struct {
		Name     string
		Input    map[string]interface{}
		Expected url.Values
		Error    string
	}
	tests := []otpTest{
		{
			Name:     "String",
			Input:    map[string]interface{}{"foo": "bar"},
			Expected: map[string][]string{"foo": {"bar"}},
		},
		{
			Name:     "StringSlice",
			Input:    map[string]interface{}{"foo": []string{"bar", "baz"}},
			Expected: map[string][]string{"foo": {"bar", "baz"}},
		},
		{
			Name:     "Bool",
			Input:    map[string]interface{}{"foo": true},
			Expected: map[string][]string{"foo": {"true"}},
		},
		{
			Name:     "Int",
			Input:    map[string]interface{}{"foo": 123},
			Expected: map[string][]string{"foo": {"123"}},
		},
		{
			Name:  "Error",
			Input: map[string]interface{}{"foo": []byte("foo")},
			Error: "cannot convert type []uint8 to []string",
		},
	}
	for _, test := range tests {
		func(test otpTest) {
			t.Run(test.Name, func(t *testing.T) {
				params, err := optionsToParams(test.Input)
				var msg string
				if err != nil {
					msg = err.Error()
				}
				if msg != test.Error {
					t.Errorf("Error\n\tExpected: %s\n\t  Actual: %s\n", test.Error, msg)
				}
				if d := diff.Interface(test.Expected, params); d != nil {
					t.Errorf("Params not as expected:\n%s\n", d)
				}
			})
		}(test)
	}
}

func TestCompact(t *testing.T) {
	tests := []struct {
		name string
		db   *db
		err  string
	}{
		{
			name: "net error",
			db:   newTestDB(nil, errors.New("net error")),
			err:  "Post http://example.com/testdb/_compact: net error",
		},
		{
			name: "1.6.1",
			db: newCustomDB(func(req *http.Request) (*http.Response, error) {
				if ct, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type")); ct != "application/json" {
					return nil, fmt.Errorf("Expected Content-Type: application/json, got %s", ct)
				}
				return &http.Response{
					StatusCode: kivik.StatusOK,
					Header: http.Header{
						"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
						"Date":           {"Thu, 26 Oct 2017 13:07:52 GMT"},
						"Content-Type":   {"text/plain; charset=utf-8"},
						"Content-Length": {"12"},
						"Cache-Control":  {"must-revalidate"},
					},
					Body: ioutil.NopCloser(strings.NewReader(`{"ok":true}`)),
				}, nil
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.db.Compact(context.Background())
			testy.Error(t, test.err, err)
		})
	}
}

func TestCompactView(t *testing.T) {
	tests := []struct {
		name   string
		db     *db
		id     string
		status int
		err    string
	}{
		{
			name:   "no ddoc",
			status: kivik.StatusBadRequest,
			err:    "kivik: ddocID required",
		},
		{
			name:   "net error",
			db:     newTestDB(nil, errors.New("net error")),
			id:     "foo",
			status: kivik.StatusInternalServerError,
			err:    "Post http://example.com/testdb/_compact/foo: net error",
		},
		{
			name: "1.6.1",
			id:   "foo",
			db: newCustomDB(func(req *http.Request) (*http.Response, error) {
				if ct, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type")); ct != "application/json" {
					return nil, fmt.Errorf("Expected Content-Type: application/json, got %s", ct)
				}
				return &http.Response{
					StatusCode: kivik.StatusAccepted,
					Header: http.Header{
						"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
						"Date":           {"Thu, 26 Oct 2017 13:07:52 GMT"},
						"Content-Type":   {"text/plain; charset=utf-8"},
						"Content-Length": {"12"},
						"Cache-Control":  {"must-revalidate"},
					},
					Body: ioutil.NopCloser(strings.NewReader(`{"ok":true}`)),
				}, nil
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.db.CompactView(context.Background(), test.id)
			testy.StatusError(t, test.err, test.status, err)
		})
	}
}

func TestViewCleanup(t *testing.T) {
	tests := []struct {
		name string
		db   *db
		err  string
	}{
		{
			name: "net error",
			db:   newTestDB(nil, errors.New("net error")),
			err:  "Post http://example.com/testdb/_view_cleanup: net error",
		},
		{
			name: "1.6.1",
			db: newCustomDB(func(req *http.Request) (*http.Response, error) {
				if ct, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type")); ct != "application/json" {
					return nil, fmt.Errorf("Expected Content-Type: application/json, got %s", ct)
				}
				return &http.Response{
					StatusCode: kivik.StatusOK,
					Header: http.Header{
						"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
						"Date":           {"Thu, 26 Oct 2017 13:07:52 GMT"},
						"Content-Type":   {"text/plain; charset=utf-8"},
						"Content-Length": {"12"},
						"Cache-Control":  {"must-revalidate"},
					},
					Body: ioutil.NopCloser(strings.NewReader(`{"ok":true}`)),
				}, nil
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.db.ViewCleanup(context.Background())
			testy.Error(t, test.err, err)
		})
	}
}

func TestPut(t *testing.T) {
	tests := []struct {
		name     string
		db       *db
		id       string
		doc      interface{}
		status   int
		rev, err string
	}{
		{
			name:   "missing docID",
			status: kivik.StatusBadRequest,
			err:    "kivik: docID required",
		},
		{
			name:   "network error",
			id:     "foo",
			db:     newTestDB(nil, errors.New("net error")),
			status: kivik.StatusInternalServerError,
			err:    "Put http://example.com/testdb/foo: net error",
		},
		{
			name: "bad request",
			id:   "foo",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusBadRequest,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}, nil),
			status: kivik.StatusBadRequest,
			err:    "Bad Request",
		},
		{
			name: "invalid JSON response",
			id:   "foo",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader("invalid json")),
			}, nil),
			status: kivik.StatusInternalServerError,
			err:    "invalid character 'i' looking for beginning of value",
		},
		{
			name: "invalid document",
			id:   "foo",
			doc:  make(chan int),
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}, nil),
			status: kivik.StatusBadRequest,
			err:    "json: unsupported type: chan int",
		},
		{
			name: "doc created, 1.6.1",
			id:   "foo",
			doc:  map[string]string{"foo": "bar"},
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusCreated,
				Header: http.Header{
					"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
					"Location":       {"http://localhost:5984/foo/foo"},
					"ETag":           {`"1-4c6114c65e295552ab1019e2b046b10e"`},
					"Date":           {"Wed, 25 Oct 2017 12:33:09 GMT"},
					"Content-Type":   {"text/plain; charset=utf-8"},
					"Content-Length": {"66"},
					"Cache-Control":  {"must-revalidate"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"ok":true,"id":"foo","rev":"1-4c6114c65e295552ab1019e2b046b10e"}`)),
			}, nil),
			rev: "1-4c6114c65e295552ab1019e2b046b10e",
		},
		{
			name: "unexpected id in response",
			id:   "foo",
			doc:  map[string]string{"foo": "bar"},
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusCreated,
				Body:       ioutil.NopCloser(strings.NewReader(`{"ok":true,"id":"unexpected","rev":"1-4c6114c65e295552ab1019e2b046b10e"}`)),
			}, nil),
			status: kivik.StatusInternalServerError,
			err:    "modified document ID (unexpected) does not match that requested (foo)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rev, err := test.db.Put(context.Background(), test.id, test.doc)
			testy.StatusError(t, test.err, test.status, err)
			if rev != test.rev {
				t.Errorf("Unexpected rev: %s", rev)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name    string
		id, rev string
		db      *db
		newrev  string
		status  int
		err     string
	}{
		{
			name:   "no doc id",
			status: kivik.StatusBadRequest,
			err:    "kivik: docID required",
		},
		{
			name:   "network error",
			id:     "foo",
			db:     newTestDB(nil, errors.New("net error")),
			status: kivik.StatusInternalServerError,
			err:    "(Delete http://example.com/testdb/foo?rev=: )?net error",
		},
		{
			name: "1.6.1 conflict",
			id:   "43734cf3ce6d5a37050c050bb600006b",
			db: newTestDB(&http.Response{
				StatusCode: 409,
				Header: http.Header{
					"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
					"Date":           {"Thu, 26 Oct 2017 13:29:06 GMT"},
					"Content-Type":   {"text/plain; charset=utf-8"},
					"Content-Length": {"58"},
					"Cache-Control":  {"must-revalidate"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"error":"conflict","reason":"Document update conflict."}`)),
			}, nil),
			status: kivik.StatusConflict,
			err:    "Conflict",
		},
		{
			name: "1.6.1 success",
			id:   "43734cf3ce6d5a37050c050bb600006b",
			rev:  "1-4c6114c65e295552ab1019e2b046b10e",
			db: newTestDB(&http.Response{
				StatusCode: 200,
				Header: http.Header{
					"Server":         {"CouchDB/1.6.1 (Erlang OTP/17)"},
					"Date":           {"Thu, 26 Oct 2017 13:29:06 GMT"},
					"Content-Type":   {"text/plain; charset=utf-8"},
					"ETag":           {`"2-185ccf92154a9f24a4f4fd12233bf463"`},
					"Content-Length": {"95"},
					"Cache-Control":  {"must-revalidate"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"ok":true,"id":"43734cf3ce6d5a37050c050bb600006b","rev":"2-185ccf92154a9f24a4f4fd12233bf463"}`)),
			}, nil),
			newrev: "2-185ccf92154a9f24a4f4fd12233bf463",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			newrev, err := test.db.Delete(context.Background(), test.id, test.rev)
			testy.StatusErrorRE(t, test.err, test.status, err)
			if newrev != test.newrev {
				t.Errorf("Unexpected new rev: %s", newrev)
			}
		})
	}
}

func TestRowsQuery(t *testing.T) {
	type queryResult struct {
		Offset    int64
		TotalRows int64
		Warning   string
		UpdateSeq string
		Err       string
		Rows      []driver.Row
	}
	tests := []struct {
		name     string
		db       *db
		path     string
		options  map[string]interface{}
		expected queryResult
		err      string
	}{
		{
			name:    "invalid options",
			path:    "_all_docs",
			options: map[string]interface{}{"foo": make(chan int)},
			err:     "cannot convert type chan int to []string",
		},
		{
			name: "network error",
			path: "_all_docs",
			db:   newTestDB(nil, errors.New("go away")),
			err:  "Get http://example.com/testdb/_all_docs: go away",
		},
		{
			name: "error response",
			path: "_all_docs",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusBadRequest,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}, nil),
			err: "Bad Request",
		},
		{
			name: "all docs default 1.6.1",
			path: "_all_docs",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Header: map[string][]string{
					"Transfer-Encoding": {"chunked"},
					"Date":              {"Tue, 24 Oct 2017 21:17:12 GMT"},
					"Server":            {"CouchDB/1.6.1 (Erlang OTP/17)"},
					"ETag":              {`"2MVNDK3T2PN4JUK89TKD10QDA"`},
					"Content-Type":      {"text/plain; charset=utf-8"},
					"Cache-Control":     {"must-revalidate"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"total_rows":3,"offset":0,"rows":[
{"id":"_design/_auth","key":"_design/_auth","value":{"rev":"1-75efcce1f083316d622d389f3f9813f7"}},
{"id":"org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye","key":"org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye","value":{"rev":"1-747e6766038164010fd0efcabd1a31dd"}},
{"id":"org.couchdb.user:zqfdn6u3cqi6pol3hslq5egiye","key":"org.couchdb.user:zqfdn6u3cqi6pol3hslq5egiye","value":{"rev":"1-4645438e6e1aa2230a1b06b5c1f5c63f"}}
]}
`)),
			}, nil),
			expected: queryResult{
				TotalRows: 3,
				Rows: []driver.Row{
					{
						ID:    "_design/_auth",
						Key:   []byte(`"_design/_auth"`),
						Value: []byte(`{"rev":"1-75efcce1f083316d622d389f3f9813f7"}`),
					},
					{
						ID:    "org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye",
						Key:   []byte(`"org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye"`),
						Value: []byte(`{"rev":"1-747e6766038164010fd0efcabd1a31dd"}`),
					},
					{
						ID:    "org.couchdb.user:zqfdn6u3cqi6pol3hslq5egiye",
						Key:   []byte(`"org.couchdb.user:zqfdn6u3cqi6pol3hslq5egiye"`),
						Value: []byte(`{"rev":"1-4645438e6e1aa2230a1b06b5c1f5c63f"}`),
					},
				},
			},
		},
		{
			name: "all docs options 1.6.1",
			path: "/_all_docs?update_seq=true&limit=1&skip=1",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Header: map[string][]string{
					"Transfer-Encoding": {"chunked"},
					"Date":              {"Tue, 24 Oct 2017 21:17:12 GMT"},
					"Server":            {"CouchDB/1.6.1 (Erlang OTP/17)"},
					"ETag":              {`"2MVNDK3T2PN4JUK89TKD10QDA"`},
					"Content-Type":      {"text/plain; charset=utf-8"},
					"Cache-Control":     {"must-revalidate"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"total_rows":3,"offset":1,"update_seq":31,"rows":[
{"id":"org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye","key":"org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye","value":{"rev":"1-747e6766038164010fd0efcabd1a31dd"}}
]}
`)),
			}, nil),
			expected: queryResult{
				TotalRows: 3,
				Offset:    1,
				UpdateSeq: "31",
				Rows: []driver.Row{
					{
						ID:    "org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye",
						Key:   []byte(`"org.couchdb.user:5wmxzru3b4i6pdmvhslq5egiye"`),
						Value: []byte(`{"rev":"1-747e6766038164010fd0efcabd1a31dd"}`),
					},
				},
			},
		},
		{
			name: "all docs options 2.0.0, no results",
			path: "/_all_docs?update_seq=true&limit=1",
			db: newTestDB(&http.Response{
				StatusCode: kivik.StatusOK,
				Header: map[string][]string{
					"Transfer-Encoding":  {"chunked"},
					"Date":               {"Tue, 24 Oct 2017 21:21:30 GMT"},
					"Server":             {"CouchDB/2.0.0 (Erlang OTP/17)"},
					"Content-Type":       {"application/json"},
					"Cache-Control":      {"must-revalidate"},
					"X-Couch-Request-ID": {"a9688d9335"},
					"X-Couch-Body-Time":  {"0"},
				},
				Body: ioutil.NopCloser(strings.NewReader(`{"total_rows":1,"offset":0,"update_seq":"13-g1AAAAEzeJzLYWBg4MhgTmHgzcvPy09JdcjLz8gvLskBCjPlsQBJhgdA6j8QZCUy4FV3AKLuflYiE151DRB18wmZtwCibj9u85ISgGRSPV63JSmA1NiD1bDgUJPIkCSP3xAHkCHxYDWsWQDg12MD","rows":[
{"id":"_design/_auth","key":"_design/_auth","value":{"rev":"1-75efcce1f083316d622d389f3f9813f7"}}
]}
`)),
			}, nil),
			expected: queryResult{
				TotalRows: 1,
				UpdateSeq: "13-g1AAAAEzeJzLYWBg4MhgTmHgzcvPy09JdcjLz8gvLskBCjPlsQBJhgdA6j8QZCUy4FV3AKLuflYiE151DRB18wmZtwCibj9u85ISgGRSPV63JSmA1NiD1bDgUJPIkCSP3xAHkCHxYDWsWQDg12MD",
				Rows: []driver.Row{
					{
						ID:    "_design/_auth",
						Key:   []byte(`"_design/_auth"`),
						Value: []byte(`{"rev":"1-75efcce1f083316d622d389f3f9813f7"}`),
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := test.db.rowsQuery(context.Background(), test.path, test.options)
			testy.Error(t, test.err, err)
			result := queryResult{
				Rows: []driver.Row{},
			}
			for {
				var row driver.Row
				if e := rows.Next(&row); e != nil {
					if e != io.EOF {
						result.Err = e.Error()
					}
					break
				}
				result.Rows = append(result.Rows, row)
			}
			result.Offset = rows.Offset()
			result.TotalRows = rows.TotalRows()
			result.UpdateSeq = rows.UpdateSeq()
			if warner, ok := rows.(driver.RowsWarner); ok {
				result.Warning = warner.Warning()
			} else {
				t.Errorf("RowsWarner interface not satisified!!?")
			}

			if d := diff.Interface(test.expected, result); d != nil {
				t.Error(d)
			}
		})
	}
}
