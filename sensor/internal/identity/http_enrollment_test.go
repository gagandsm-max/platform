package identity

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPEnrollmentTransportUsesCallerSuppliedPathAndStrictTLS(t *testing.T) {
	now := time.Now().UTC(); enrollment := makeEnrollmentResponse(t,"sensor-http",now.Add(-time.Minute),now.Add(time.Hour)); var received enrollmentWireRequest
	server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.URL.Path!="/contract-supplied/enroll"{t.Errorf("unexpected path %s",r.URL.Path)};if err:=json.NewDecoder(r.Body).Decode(&received);err!=nil{t.Errorf("decode request: %v",err)};_ = json.NewEncoder(w).Encode(enrollmentWireResponse{SensorID:enrollment.SensorID,CertificatePEM:string(enrollment.CertificatePEM),PrivateKeyPEM:string(enrollment.PrivateKeyPEM),CABundlePEM:string(enrollment.CABundlePEM)})}));defer server.Close()
	caPEM:=pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:server.Certificate().Raw});caFile:=filepath.Join(t.TempDir(),"server-ca.crt");if err:=os.WriteFile(caFile,caPEM,0600);err!=nil{t.Fatal(err)}
	transport,err:=NewHTTPEnrollmentTransport(server.URL+"/contract-supplied/enroll",caFile,time.Second,1<<20);if err!=nil{t.Fatal(err)};response,err:=transport.Enroll(context.Background(),[]byte("test-only-token"),EnrollmentRequest{Role:"NETWORK",SiteID:"site-a"});if err!=nil{t.Fatal(err)};if response.SensorID!="sensor-http"||received.Token!="test-only-token"||received.SiteID!="site-a"{t.Fatalf("response=%+v request=%+v",response,received)}
}

func TestHTTPEnrollmentTransportRejectsHTTPAndUntrustedTLS(t *testing.T){if _,err:=NewHTTPEnrollmentTransport("http://core.test/enroll","/missing",time.Second,1024);err==nil{t.Fatal("HTTP enrollment endpoint accepted")};server:=httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter,*http.Request){}));defer server.Close();otherCert,_:=x509.ParseCertificate(server.Certificate().Raw);_ = otherCert;badCA:=filepath.Join(t.TempDir(),"bad-ca.crt");_ = os.WriteFile(badCA,[]byte("not a certificate"),0600);if _,err:=NewHTTPEnrollmentTransport(server.URL+"/enroll",badCA,time.Second,1024);err==nil{t.Fatal("invalid CA accepted")}}
func TestHTTPEnrollmentTransportErrorNeverContainsToken(t *testing.T){server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){http.Error(w,"denied",http.StatusUnauthorized)}));defer server.Close();caFile:=filepath.Join(t.TempDir(),"ca.crt");_ = os.WriteFile(caFile,pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:server.Certificate().Raw}),0600);transport,err:=NewHTTPEnrollmentTransport(server.URL+"/enroll",caFile,time.Second,1024);if err!=nil{t.Fatal(err)};_,err=transport.Enroll(context.Background(),[]byte("never-log-this-token"),EnrollmentRequest{Role:"NETWORK"});if err==nil||strings.Contains(err.Error(),"never-log-this-token"){t.Fatalf("error=%v",err)}}
func TestHTTPEnrollmentTransportResponseBound(t *testing.T){server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){_,_=w.Write([]byte(strings.Repeat("x",128)))}));defer server.Close();caFile:=filepath.Join(t.TempDir(),"ca.crt");_ = os.WriteFile(caFile,pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:server.Certificate().Raw}),0600);transport,err:=NewHTTPEnrollmentTransport(server.URL+"/enroll",caFile,time.Second,32);if err!=nil{t.Fatal(err)};if _,err:=transport.Enroll(context.Background(),[]byte("token"),EnrollmentRequest{Role:"NETWORK"});err==nil||!strings.Contains(err.Error(),"TOO_LARGE"){t.Fatalf("error=%v",err)}}
