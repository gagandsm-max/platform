package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeEnrollmentClient struct { response EnrollmentResponse; err error; seenToken string; calls int }
func (f *fakeEnrollmentClient) Enroll(_ context.Context,token []byte,_ EnrollmentRequest)(EnrollmentResponse,error){f.calls++;f.seenToken=string(token);return f.response,f.err}

func makeEnrollmentResponse(t *testing.T,sensorID string,notBefore,notAfter time.Time) EnrollmentResponse {
	t.Helper();caKey,_:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);caTemplate:=&x509.Certificate{SerialNumber:big.NewInt(1),Subject:pkix.Name{CommonName:"test-ca"},NotBefore:notBefore.Add(-time.Hour),NotAfter:notAfter.Add(time.Hour),IsCA:true,BasicConstraintsValid:true,KeyUsage:x509.KeyUsageCertSign|x509.KeyUsageDigitalSignature};caDER,err:=x509.CreateCertificate(rand.Reader,caTemplate,caTemplate,&caKey.PublicKey,caKey);if err!=nil{t.Fatal(err)};caCert,_:=x509.ParseCertificate(caDER);key,_:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);leaf:=&x509.Certificate{SerialNumber:big.NewInt(2),Subject:pkix.Name{CommonName:sensorID},NotBefore:notBefore,NotAfter:notAfter,KeyUsage:x509.KeyUsageDigitalSignature,ExtKeyUsage:[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}};leafDER,err:=x509.CreateCertificate(rand.Reader,leaf,caCert,&key.PublicKey,caKey);if err!=nil{t.Fatal(err)};keyDER,_:=x509.MarshalPKCS8PrivateKey(key);return EnrollmentResponse{SensorID:sensorID,CertificatePEM:pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:leafDER}),PrivateKeyPEM:pem.EncodeToMemory(&pem.Block{Type:"PRIVATE KEY",Bytes:keyDER}),CABundlePEM:pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:caDER})}
}
func newTestStore(t *testing.T,now time.Time)*Store{t.Helper();store,err:=NewStore(filepath.Join(t.TempDir(),"identity"));if err!=nil{t.Fatal(err)};store.now=func()time.Time{return now};return store}

func TestBootstrapTokenOneTimeAndRedacted(t *testing.T){token,err:=NewBootstrapToken([]byte("test-only-token"));if err!=nil{t.Fatal(err)};for _,rendered:=range []string{fmt.Sprintf("%v",token),fmt.Sprintf("%+v",token),fmt.Sprintf("%#v",token)}{if strings.Contains(rendered,"test-only-token"){t.Fatalf("token leaked: %s",rendered)}};value,err:=token.Consume();if err!=nil||string(value)!="test-only-token"{t.Fatalf("value=%q err=%v",string(value),err)};ZeroBytes(value);if _,err:=token.Consume();err==nil{t.Fatal("token reused")}}
func TestEnrollConsumesTokenAndPersistsIdentity(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);response:=makeEnrollmentResponse(t,"sensor-001",now.Add(-time.Minute),now.Add(time.Hour));client:=&fakeEnrollmentClient{response:response};token,_:=NewBootstrapToken([]byte("test-only-bootstrap"));identity,err:=store.Enroll(context.Background(),client,token,EnrollmentRequest{Role:"NETWORK"});if err!=nil{t.Fatal(err)};if client.calls!=1||client.seenToken!="test-only-bootstrap"||identity.SensorID!="sensor-001"{t.Fatalf("client=%+v identity=%+v",client,identity)};if _,err:=token.Consume();err==nil{t.Fatal("token remained reusable after enrollment")};loaded,err:=store.Load();if err!=nil{t.Fatal(err)};if loaded.CertificateSHA256!=identity.CertificateSHA256{t.Fatalf("loaded=%+v identity=%+v",loaded,identity)}}
func TestPrivateKeyPermissionsAreRestricted(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);identity,err:=store.Persist(makeEnrollmentResponse(t,"sensor-001",now.Add(-time.Minute),now.Add(time.Hour)));if err!=nil{t.Fatal(err)};_,key,_,err:=store.Paths(identity);if err!=nil{t.Fatal(err)};info,err:=os.Stat(key);if err!=nil{t.Fatal(err)};if info.Mode().Perm()!=0600{t.Fatalf("private key mode=%o",info.Mode().Perm())};if err:=os.Chmod(key,0644);err!=nil{t.Fatal(err)};if _,err:=store.Load();err==nil||!strings.Contains(err.Error(),"KEY_PERMISSIONS"){t.Fatalf("error=%v",err)}}
func TestCertificateExpiryStates(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);identity,err:=store.Persist(makeEnrollmentResponse(t,"sensor-001",now.Add(-time.Minute),now.Add(2*time.Hour)));if err!=nil{t.Fatal(err)};if got:=store.Status(identity,30*time.Minute).State;got!=CertificateValid{t.Fatalf("state=%s",got)};if got:=store.Status(identity,3*time.Hour).State;got!=CertificateExpiring{t.Fatalf("state=%s",got)};store.now=func()time.Time{return now.Add(3*time.Hour)};if got:=store.Status(identity,30*time.Minute).State;got!=CertificateExpired{t.Fatalf("state=%s",got)}}
func TestEnrollmentRejectsMismatchedCertificateIdentity(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);response:=makeEnrollmentResponse(t,"sensor-cert-name",now.Add(-time.Minute),now.Add(time.Hour));response.SensorID="sensor-other";if _,err:=store.Persist(response);err==nil||!strings.Contains(err.Error(),"CERTIFICATE_MISMATCH"){t.Fatalf("error=%v",err)}}
func TestEnrollmentRejectsExpiredCertificateAndInvalidCA(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);expired:=makeEnrollmentResponse(t,"sensor-001",now.Add(-2*time.Hour),now.Add(-time.Hour));if _,err:=store.Persist(expired);err==nil||!strings.Contains(err.Error(),"CERTIFICATE_INVALID"){t.Fatalf("expired error=%v",err)};valid:=makeEnrollmentResponse(t,"sensor-001",now.Add(-time.Minute),now.Add(time.Hour));valid.CABundlePEM=[]byte("invalid");if _,err:=store.Persist(valid);err==nil||!strings.Contains(err.Error(),"CA_INVALID"){t.Fatalf("ca error=%v",err)}}
func TestLoadRejectsPathTraversalMetadata(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);metadata:=Identity{SensorID:"sensor-001",CertificateFile:"../cert",PrivateKeyFile:"key",CAFile:"ca",CertificateSHA256:"x"};data,_:=json.Marshal(metadata);if err:=os.WriteFile(filepath.Join(store.dir,metadataFile),data,0600);err!=nil{t.Fatal(err)};if _,err:=store.Load();err==nil||!strings.Contains(err.Error(),"PATH_UNSAFE"){t.Fatalf("error=%v",err)}}
func TestLoadRejectsSymlinkIdentityFiles(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);identity,err:=store.Persist(makeEnrollmentResponse(t,"sensor-001",now.Add(-time.Minute),now.Add(time.Hour)));if err!=nil{t.Fatal(err)};cert,_,_,_:=store.Paths(identity);target:=filepath.Join(t.TempDir(),"other.crt");_ = os.WriteFile(target,[]byte("x"),0600);_ = os.Remove(cert);if err:=os.Symlink(target,cert);err!=nil{t.Skipf("symlink unavailable: %v",err)};if _,err:=store.Load();err==nil||!strings.Contains(err.Error(),"PATH_UNSAFE"){t.Fatalf("error=%v",err)}}
func TestCrashStylePartialGenerationDoesNotReplaceCommittedMetadata(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);first,err:=store.Persist(makeEnrollmentResponse(t,"sensor-001",now.Add(-time.Minute),now.Add(time.Hour)));if err!=nil{t.Fatal(err)};if err:=os.WriteFile(filepath.Join(store.dir,"sensor-partial.crt"),[]byte("partial"),0640);err!=nil{t.Fatal(err)};if err:=os.WriteFile(filepath.Join(store.dir,"sensor-partial.key"),[]byte("partial"),0600);err!=nil{t.Fatal(err)};loaded,err:=store.Load();if err!=nil{t.Fatal(err)};if loaded.CertificateSHA256!=first.CertificateSHA256{t.Fatal("partial generation displaced committed identity")}}
func TestMetadataUnknownFieldRejected(t *testing.T){now:=time.Now().UTC();store:=newTestStore(t,now);_ = os.WriteFile(filepath.Join(store.dir,metadataFile),[]byte(`{"sensorId":"x","unexpected":true}`),0600);if _,err:=store.Load();err==nil||!strings.Contains(err.Error(),"METADATA_INVALID"){t.Fatalf("error=%v",err)}}
