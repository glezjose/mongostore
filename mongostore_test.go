package mongostore_test

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/glezjose/mongostore"
	"github.com/gorilla/securecookie"
)

var err error
var mongoclient *mongo.Client
var store *mongostore.Store

func TestMain(m *testing.M) {
	// connect to database
	testsSetup()

	// run all tests
	ret := m.Run()

	// disconnect from database
	testsTeardown()

	// call flag.Parse() here if TestMain uses flags
	os.Exit(ret)
}

func testsSetup() {
	// if environment variable is does not exist or is empty set a default
	if os.Getenv("MONGODB_URI") == "" {
		os.Setenv("MONGODB_URI", "mongodb://localhost:27017")
	}

	// A Context carries a deadline, cancelation signal, and request-scoped values
	// across API boundaries. Its methods are safe for simultaneous use by multiple
	// goroutines.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect does not do server discovery, use Ping method.
	mongoclient, err = mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("MONGODB_URI")))
	if err != nil {
		log.Fatal(err)
	}

	// Ping for server discovery.
	err = mongoclient.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatal(err)
	}
}

func testsTeardown() {
	err = mongoclient.Disconnect(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}

func TestNewStore(t *testing.T) {
	// without environment variables
	// os.Clearenv()
	os.Setenv("GORILLA_SESSION_AUTH_KEY", "")
	os.Setenv("GORILLA_SESSION_ENC_KEY", "")

	// get a new store
	store, err = mongostore.NewStore(
		mongoclient.Database("test-database").Collection("sessions_test"),
		http.Cookie{
			Path:     "/",
			Domain:   "",
			MaxAge:   240,
			Secure:   false,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		},
		[]byte(os.Getenv("GORILLA_SESSION_AUTH_KEY")),
		[]byte(os.Getenv("GORILLA_SESSION_ENC_KEY")),
	)

	if err != nil {
		t.Fatal(err)
	}

	if store == nil {
		t.Fatal("expected to fail with no environment variables")
	}

	// without TTL index
	_, err = mongoclient.Database("test-database").Collection("sessions_test").Indexes().DropAll(context.TODO())
	if err != nil {
		t.Fatalf("failed to drop mongo indexes: %v\n", err)
	}

	// with environment variables
	// if environment variable is does not exist or is empty set a default
	if os.Getenv("GORILLA_SESSION_AUTH_KEY") == "" {
		os.Setenv("GORILLA_SESSION_AUTH_KEY", string(securecookie.GenerateRandomKey(32)))
	}

	// if environment variable is does not exist or is empty set a default
	if os.Getenv("GORILLA_SESSION_ENC_KEY") == "" {
		os.Setenv("GORILLA_SESSION_ENC_KEY", string(securecookie.GenerateRandomKey(16)))
	}

	// get a new store
	store, err = mongostore.NewStore(
		mongoclient.Database("test-database").Collection("sessions_test"),
		http.Cookie{
			Path:     "/",
			Domain:   "",
			MaxAge:   240,
			Secure:   false,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		},
		[]byte(os.Getenv("GORILLA_SESSION_AUTH_KEY")),
		[]byte(os.Getenv("GORILLA_SESSION_ENC_KEY")),
	)

	if err != nil {
		t.Fatal(err)
	}

	// if store is nil, throw an error
	if store == nil {
		t.Fatal("failed to create new store")
	}
}

func TestGet(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)

	_, err := store.Get(req, "test-session")
	if err != nil {
		t.Fatalf("failed to get session: %v\n", err)
	}
}

func TestNew(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)

	// new session
	_, err = store.New(req, "test-session")
	if err != nil {
		t.Fatalf("failed to create new session: %v\n", err)
	}

	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)

	// existing session
	_, err = store.New(req, "test-session")
	if err != nil {
		t.Fatalf("failed to create new session: %v\n", err)
	}
}

func TestSave(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
	res := httptest.NewRecorder()

	// insert mongo
	session, err := store.Get(req, "test-session")
	if err != nil {
		t.Fatalf("failed to get session: %v\n", err)
	}
	session.Values["test"] = "testdata"
	err = store.Save(req, res, session)
	if err != nil {
		t.Fatalf("failed to insert session: %v\n", err)
	}

	// insert cookie
	hdr := res.Header()
	cookies, ok := hdr["Set-Cookie"]
	if !ok || len(cookies) != 1 {
		t.Fatal("no cookies. header:", hdr)
	}

	// update mongo
	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)
	req.Header.Add("Cookie", cookies[0])
	res = httptest.NewRecorder()

	session, err = store.Get(req, "test-session")
	if err != nil {
		t.Fatalf("failed to get session: %v\n", err)
	}
	session.Options.MaxAge = 7357
	err = store.Save(req, res, session)
	if err != nil {
		t.Fatal("failed to update session", err)
	}

	// insert cookie
	hdr = res.Header()
	cookies, ok = hdr["Set-Cookie"]
	if !ok || len(cookies) != 1 {
		t.Fatal("no cookies. header:", hdr)
	}

	// expire mongo
	req, _ = http.NewRequest("GET", "http://localhost:8080/", nil)
	req.Header.Add("Cookie", cookies[0])
	res = httptest.NewRecorder()

	session, err = store.Get(req, "test-session")
	if err != nil {
		t.Fatalf("failed to get session: %v\n", err)
	}
	session.Options.MaxAge = -1
	err = store.Save(req, res, session)
	if err != nil {
		t.Fatal("failed to expire session", err)
	}

}

// func TestMaxAge(t *testing.T) {
// 	store.MaxAge(7357)
// 	if store.Options.MaxAge != 7357 {
// 		t.Fatalf("failed to set MaxAge: %v\n", err)
// 	}
// }
