package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	metadataFile     = "identity.json"
	maxMetadataBytes = 64 << 10
)

type BootstrapToken struct {
	mu       sync.Mutex
	value    []byte
	consumed bool
}

func NewBootstrapToken(value []byte) (*BootstrapToken, error) {
	if len(value) == 0 { return nil, fmt.Errorf("SENSOR_IDENTITY_TOKEN_INVALID: bootstrap token is empty") }
	return &BootstrapToken{value: append([]byte(nil), value...)}, nil
}
func (t *BootstrapToken) String() string { return "[REDACTED]" }
func (t *BootstrapToken) GoString() string { return "[REDACTED]" }
func (t *BootstrapToken) Consume() ([]byte, error) {
	if t == nil { return nil, fmt.Errorf("SENSOR_IDENTITY_TOKEN_INVALID: bootstrap token is nil") }
	t.mu.Lock(); defer t.mu.Unlock()
	if t.consumed { return nil, fmt.Errorf("SENSOR_IDENTITY_TOKEN_CONSUMED: bootstrap token has already been consumed") }
	t.consumed = true; out := append([]byte(nil), t.value...); ZeroBytes(t.value); t.value = nil; return out, nil
}
func ZeroBytes(value []byte) { for i := range value { value[i] = 0 } }

type EnrollmentRequest struct { Role string `json:"role"`; SiteID string `json:"siteId,omitempty"`; PoolID string `json:"poolId,omitempty"` }
type EnrollmentResponse struct { SensorID string; CertificatePEM []byte; PrivateKeyPEM []byte; CABundlePEM []byte }
type EnrollmentClient interface { Enroll(context.Context, []byte, EnrollmentRequest) (EnrollmentResponse, error) }

type Identity struct {
	SensorID string `json:"sensorId"`
	CertificateFile string `json:"certificateFile"`
	PrivateKeyFile string `json:"privateKeyFile"`
	CAFile string `json:"caFile"`
	CertificateSHA256 string `json:"certificateSha256"`
	CertificateNotBefore time.Time `json:"certificateNotBefore"`
	CertificateNotAfter time.Time `json:"certificateNotAfter"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CertificateState string
const ( CertificateValid CertificateState="VALID"; CertificateExpiring CertificateState="EXPIRING"; CertificateExpired CertificateState="EXPIRED"; CertificateInvalid CertificateState="INVALID" )
type Status struct { State CertificateState; NotAfter time.Time; Remaining time.Duration }

type Store struct { dir string; now func() time.Time; mu sync.Mutex }

func NewStore(dir string) (*Store,error) {
	if dir==""||!filepath.IsAbs(dir){return nil,fmt.Errorf("SENSOR_IDENTITY_PATH_INVALID: identity directory must be absolute")}
	if err:=ensureSecureDirectory(dir);err!=nil{return nil,err}
	return &Store{dir:dir,now:time.Now},nil
}
func ensureSecureDirectory(dir string) error {
	if info,err:=os.Lstat(dir);err==nil{
		if info.Mode()&os.ModeSymlink!=0||!info.IsDir(){return fmt.Errorf("SENSOR_IDENTITY_PATH_UNSAFE: identity path must be a real directory")}
		if err:=os.Chmod(dir,0700);err!=nil{return fmt.Errorf("SENSOR_IDENTITY_PATH_PERMISSIONS: %w",err)};return nil
	}else if !errors.Is(err,os.ErrNotExist){return fmt.Errorf("SENSOR_IDENTITY_PATH_UNAVAILABLE: %w",err)}
	if err:=os.MkdirAll(dir,0700);err!=nil{return fmt.Errorf("SENSOR_IDENTITY_PATH_CREATE_FAILED: %w",err)};return os.Chmod(dir,0700)
}

func (s *Store) Enroll(ctx context.Context, client EnrollmentClient, token *BootstrapToken, req EnrollmentRequest)(Identity,error){
	if s==nil||client==nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_ENROLLMENT_INVALID: store and enrollment client are required")}
	if ctx==nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_ENROLLMENT_INVALID: context is required")}
	tokenBytes,err:=token.Consume();if err!=nil{return Identity{},err};defer ZeroBytes(tokenBytes)
	response,err:=client.Enroll(ctx,tokenBytes,req);if err!=nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_ENROLLMENT_FAILED: %w",err)}
	return s.Persist(response)
}

func (s *Store) Persist(response EnrollmentResponse)(Identity,error){
	if s==nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_STORE_INVALID: store is nil")}
	s.mu.Lock();defer s.mu.Unlock()
	identity,certDER,err:=validateEnrollmentResponse(response,s.now());if err!=nil{return Identity{},err}
	sum:=sha256.Sum256(certDER);generation:=hex.EncodeToString(sum[:12]);certName:="sensor-"+generation+".crt";keyName:="sensor-"+generation+".key";caName:="ca-"+generation+".crt"
	identity.CertificateFile=certName;identity.PrivateKeyFile=keyName;identity.CAFile=caName;identity.CertificateSHA256=hex.EncodeToString(sum[:]);identity.UpdatedAt=s.now().UTC()
	if err:=atomicWrite(filepath.Join(s.dir,keyName),response.PrivateKeyPEM,0600);err!=nil{return Identity{},err}
	if err:=atomicWrite(filepath.Join(s.dir,certName),response.CertificatePEM,0640);err!=nil{return Identity{},err}
	if err:=atomicWrite(filepath.Join(s.dir,caName),response.CABundlePEM,0640);err!=nil{return Identity{},err}
	metadata,err:=json.Marshal(identity);if err!=nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_METADATA_ENCODE_FAILED: %w",err)}
	if err:=atomicWrite(filepath.Join(s.dir,metadataFile),metadata,0600);err!=nil{return Identity{},err}
	if err:=syncDir(s.dir);err!=nil{return Identity{},err};return identity,nil
}

func validateEnrollmentResponse(response EnrollmentResponse, now time.Time)(Identity,[]byte,error){
	if strings.TrimSpace(response.SensorID)==""{return Identity{},nil,fmt.Errorf("SENSOR_IDENTITY_RESPONSE_INVALID: Sensor ID is required")}
	pair,err:=tls.X509KeyPair(response.CertificatePEM,response.PrivateKeyPEM);if err!=nil{return Identity{},nil,fmt.Errorf("SENSOR_IDENTITY_KEYPAIR_INVALID: certificate/private key do not form a valid keypair")}
	if len(pair.Certificate)==0{return Identity{},nil,fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_INVALID: client certificate is missing")}
	cert,err:=x509.ParseCertificate(pair.Certificate[0]);if err!=nil{return Identity{},nil,fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_INVALID: client certificate cannot be parsed")}
	if now.Before(cert.NotBefore)||!now.Before(cert.NotAfter){return Identity{},nil,fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_INVALID: client certificate is not currently valid")}
	if cert.Subject.CommonName!=""&&cert.Subject.CommonName!=response.SensorID{return Identity{},nil,fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_MISMATCH: certificate common name does not match Sensor ID")}
	roots:=x509.NewCertPool();if !roots.AppendCertsFromPEM(response.CABundlePEM){return Identity{},nil,fmt.Errorf("SENSOR_IDENTITY_CA_INVALID: trust bundle contains no usable CA certificate")}
	return Identity{SensorID:response.SensorID,CertificateNotBefore:cert.NotBefore.UTC(),CertificateNotAfter:cert.NotAfter.UTC()},pair.Certificate[0],nil
}

func (s *Store) Load()(Identity,error){
	if s==nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_STORE_INVALID: store is nil")};s.mu.Lock();defer s.mu.Unlock()
	data,err:=readBoundedFile(filepath.Join(s.dir,metadataFile),maxMetadataBytes);if err!=nil{return Identity{},err}
	decoder:=json.NewDecoder(bytes.NewReader(data));decoder.DisallowUnknownFields();var identity Identity
	if err:=decoder.Decode(&identity);err!=nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_METADATA_INVALID: %w",err)};var trailing any;if err:=decoder.Decode(&trailing);err!=io.EOF{return Identity{},fmt.Errorf("SENSOR_IDENTITY_METADATA_INVALID: trailing data")}
	if identity.SensorID==""||identity.CertificateFile==""||identity.PrivateKeyFile==""||identity.CAFile==""{return Identity{},fmt.Errorf("SENSOR_IDENTITY_METADATA_INVALID: required identity fields are missing")}
	certPath,err:=s.safePath(identity.CertificateFile);if err!=nil{return Identity{},err};keyPath,err:=s.safePath(identity.PrivateKeyFile);if err!=nil{return Identity{},err};caPath,err:=s.safePath(identity.CAFile);if err!=nil{return Identity{},err}
	if err:=rejectSymlink(certPath);err!=nil{return Identity{},err};if err:=rejectSymlink(keyPath);err!=nil{return Identity{},err};if err:=rejectSymlink(caPath);err!=nil{return Identity{},err}
	keyInfo,err:=os.Stat(keyPath);if err!=nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_KEY_UNAVAILABLE: %w",err)};if keyInfo.Mode().Perm()&0077!=0{return Identity{},fmt.Errorf("SENSOR_IDENTITY_KEY_PERMISSIONS: private key permissions are too broad")}
	certPEM,err:=os.ReadFile(certPath);if err!=nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_UNAVAILABLE: %w",err)};keyPEM,err:=os.ReadFile(keyPath);if err!=nil{return Identity{},fmt.Errorf("SENSOR_IDENTITY_KEY_UNAVAILABLE: %w",err)}
	pair,err:=tls.X509KeyPair(certPEM,keyPEM);if err!=nil||len(pair.Certificate)==0{return Identity{},fmt.Errorf("SENSOR_IDENTITY_KEYPAIR_INVALID: persisted certificate/private key are invalid")};sum:=sha256.Sum256(pair.Certificate[0]);if hex.EncodeToString(sum[:])!=identity.CertificateSHA256{return Identity{},fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_INTEGRITY: persisted certificate does not match metadata fingerprint")};return identity,nil
}

func (s *Store) Status(identity Identity,rotateBefore time.Duration) Status { now:=s.now();remaining:=identity.CertificateNotAfter.Sub(now);switch{case !now.Before(identity.CertificateNotAfter):return Status{State:CertificateExpired,NotAfter:identity.CertificateNotAfter,Remaining:remaining};case rotateBefore>0&&remaining<=rotateBefore:return Status{State:CertificateExpiring,NotAfter:identity.CertificateNotAfter,Remaining:remaining};default:return Status{State:CertificateValid,NotAfter:identity.CertificateNotAfter,Remaining:remaining}} }
func (s *Store) Paths(identity Identity)(cert,key,ca string,err error){if cert,err=s.safePath(identity.CertificateFile);err!=nil{return "","","",err};if key,err=s.safePath(identity.PrivateKeyFile);err!=nil{return "","","",err};if ca,err=s.safePath(identity.CAFile);err!=nil{return "","","",err};return}
func (s *Store) safePath(name string)(string,error){if name==""||filepath.IsAbs(name)||filepath.Base(name)!=name||strings.Contains(name,".."){return "",fmt.Errorf("SENSOR_IDENTITY_PATH_UNSAFE: identity metadata contains unsafe path")};path:=filepath.Join(s.dir,name);rel,err:=filepath.Rel(s.dir,path);if err!=nil||strings.HasPrefix(rel,".."){return "",fmt.Errorf("SENSOR_IDENTITY_PATH_UNSAFE: identity path escapes store")};return path,nil}
func rejectSymlink(path string)error{info,err:=os.Lstat(path);if err!=nil{return fmt.Errorf("SENSOR_IDENTITY_FILE_UNAVAILABLE: %w",err)};if info.Mode()&os.ModeSymlink!=0||!info.Mode().IsRegular(){return fmt.Errorf("SENSOR_IDENTITY_PATH_UNSAFE: identity file must be a regular non-symlink file")};return nil}
func atomicWrite(path string,data []byte,mode os.FileMode)error{dir:=filepath.Dir(path);tmp,err:=os.CreateTemp(dir,".identity-*");if err!=nil{return fmt.Errorf("SENSOR_IDENTITY_WRITE_FAILED: %w",err)};tmpName:=tmp.Name();ok:=false;defer func(){_ = tmp.Close();if !ok{_ = os.Remove(tmpName)}}();if err:=tmp.Chmod(mode);err!=nil{return fmt.Errorf("SENSOR_IDENTITY_WRITE_FAILED: %w",err)};if _,err:=tmp.Write(data);err!=nil{return fmt.Errorf("SENSOR_IDENTITY_WRITE_FAILED: %w",err)};if err:=tmp.Sync();err!=nil{return fmt.Errorf("SENSOR_IDENTITY_WRITE_FAILED: %w",err)};if err:=tmp.Close();err!=nil{return fmt.Errorf("SENSOR_IDENTITY_WRITE_FAILED: %w",err)};if err:=os.Rename(tmpName,path);err!=nil{return fmt.Errorf("SENSOR_IDENTITY_WRITE_FAILED: %w",err)};ok=true;return nil}
func syncDir(dir string)error{f,err:=os.Open(dir);if err!=nil{return fmt.Errorf("SENSOR_IDENTITY_SYNC_FAILED: %w",err)};defer f.Close();if err:=f.Sync();err!=nil{return fmt.Errorf("SENSOR_IDENTITY_SYNC_FAILED: %w",err)};return nil}
func readBoundedFile(path string,max int64)([]byte,error){f,err:=os.Open(path);if err!=nil{return nil,fmt.Errorf("SENSOR_IDENTITY_METADATA_UNAVAILABLE: %w",err)};defer f.Close();data,err:=io.ReadAll(io.LimitReader(f,max+1));if err!=nil{return nil,fmt.Errorf("SENSOR_IDENTITY_METADATA_UNAVAILABLE: %w",err)};if int64(len(data))>max{return nil,fmt.Errorf("SENSOR_IDENTITY_METADATA_INVALID: metadata exceeds bound")};return data,nil}
func ParseFirstCertificate(pemBytes []byte)(*x509.Certificate,error){block,_:=pem.Decode(pemBytes);if block==nil||block.Type!="CERTIFICATE"{return nil,fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_INVALID: PEM certificate not found")};cert,err:=x509.ParseCertificate(block.Bytes);if err!=nil{return nil,fmt.Errorf("SENSOR_IDENTITY_CERTIFICATE_INVALID: %w",err)};return cert,nil}
