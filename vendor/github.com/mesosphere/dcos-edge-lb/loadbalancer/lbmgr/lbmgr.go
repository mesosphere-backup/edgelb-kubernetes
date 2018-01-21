package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	flags "github.com/jessevdk/go-flags"
	apisUtil "github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

type lbState struct {
	hapConfig     string
	pidFile       string
	apisReadyURL  string
	poolConfigURL string
	hapExe        string
	iptablesExe   string
	shellExe      string
	ports         []int
	hapStateDir   string
	hapWrapper    string
	hapWorkDir    string
}

type shared struct {
	// With defaults
	HapWorkDir string `long:"hapworkdir" default:"/var/run/haproxy" env:"HAPROXY_WORKDIR" description:"Haproxy work dir"`
}

type runCommand struct {
	shared

	// Without defaults and required
	WorkDir        string `long:"workdir" required:"true" env:"LBWORKDIR" description:"Work dir"`
	Ports          []int  `long:"ports" required:"true" env:"PORTS" env-delim:"," description:"Ports"`
	PoolName       string `long:"poolname" required:"true" env:"ELB_POOL_NAME" description:"Pool name"`
	PersistDir     string `long:"persistdir" env:"LBMGR_DIR" description:"Path to persistent work dir"`
	SecretsRelPath string `long:"srpath" env:"SECRETS_RELATIVE_PATH" description:"Relative path to secrets"`
	EnvfileRelPath string `long:"efrpath" env:"ENVFILE_RELATIVE_PATH" description:"Relative path to env files"`
	SecretsDir     string `long:"sdir" env:"SECRETS_DIR" description:"Absolute path to secrets"`
	EnvfileDir     string `long:"efdir" env:"ENVFILE_DIR" description:"Absolute path to env files"`
	MesosSandbox   string `long:"msandbox" env:"MESOS_SANDBOX" description:"Mesos sandbox"`

	// With defaults
	HapStateDir string `long:"hapstatedir" default:"/var/state/haproxy" env:"HAPROXY_STATEDIR" description:"Haproxy state dir"`
	ApisURL     string `long:"apisurl" default:"http://api.edgelb.marathon.l4lb.thisdcos.directory:80" env:"APIS_URL" description:"Apiserver URL"`
	AutoCert    bool   `long:"autocert" env:"AUTOCERT" description:"Generate a self-signed certificate"`
}

type healthCheckCommand struct {
	shared
}

type cliOptions struct {
	Run         runCommand         `command:"run"`
	HealthCheck healthCheckCommand `command:"healthcheck"`
}

const (
	envToFilePrefix   string        = "ELB_FILE_"
	defaultDirMode    os.FileMode   = 0755
	defaultFileMode   os.FileMode   = 0644
	defaultReloadWait time.Duration = time.Second * 5
	defaultFastWait   time.Duration = time.Millisecond * 100
	pongMessage       string        = "pong"

	secretsVarName     string = "SECRETS"
	envfileVarName     string = "ENVFILE"
	autocertpemVarName string = "AUTOCERT"

	autocertpemFile string = "autocert.pem"
	autocertcrtFile string = "autocert.crt"
	autocertkeyFile string = "autocert.key"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)

	var opts cliOptions
	parser := flags.NewParser(&opts, flags.Default)

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			log.Fatal(err)
		}
	}
}

func (opts *healthCheckCommand) Execute(args []string) error {
	hapSocket := fmt.Sprintf("%s/socket", opts.HapWorkDir)
	cmdStr := fmt.Sprintf(`socat %s - <<< "show servers state"`, hapSocket)
	shellExe := mustGetPath("bash")
	b, err := execRun(exec.Command(shellExe, "-c", cmdStr))
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return fmt.Errorf("health check failed")
	}
	return nil
}

func (opts *runCommand) Execute(args []string) error {
	// This function must be idempotent. That is because all of this code is
	// re-run whenever it crashes.

	if err := os.Setenv(secretsVarName, opts.SecretsRelPath); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv(envfileVarName, opts.EnvfileRelPath); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv(autocertpemVarName, autocertpemFile); err != nil {
		log.Fatal(err)
	}

	if err := maybeEnvToFile(opts.EnvfileDir, envToFilePrefix); err != nil {
		log.Fatalf("failed env to file: %s", err)
	}

	if err := maybeGenCert(opts.AutoCert, opts.MesosSandbox); err != nil {
		log.Fatalf("failed gen cert: %s", err)
	}

	hapWrapper := fmt.Sprintf("%s/haproxy_wrapper.py", opts.WorkDir)
	hapConfig := fmt.Sprintf("%s/haproxy.cfg", opts.PersistDir)
	pidFile := fmt.Sprintf("%s/haproxy.pid", opts.HapWorkDir)
	apisReadyURL := fmt.Sprintf("%s/ping", opts.ApisURL)
	poolConfigURL := fmt.Sprintf("%s/v2/pools/%s/lbconfig", opts.ApisURL, opts.PoolName)

	if err := os.MkdirAll(opts.HapStateDir, defaultDirMode); err != nil {
		log.Fatalf("error mkdir state dir: %s", err)
	}
	if err := os.MkdirAll(opts.HapWorkDir, defaultDirMode); err != nil {
		log.Fatalf("error mkdir haproxy work dir: %s", err)
	}

	lb := &lbState{
		hapConfig:     hapConfig,
		pidFile:       pidFile,
		apisReadyURL:  apisReadyURL,
		poolConfigURL: poolConfigURL,
		hapExe:        mustGetPath("haproxy"),
		iptablesExe:   mustGetPath("iptables"),
		shellExe:      mustGetPath("bash"),
		ports:         opts.Ports,
		hapStateDir:   opts.HapStateDir,
		hapWrapper:    hapWrapper,
		hapWorkDir:    opts.HapWorkDir,
	}
	log.Printf("%+v", lb)

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGHUP)

	// Try a reload before anything else, this is so that if there was a config
	// already on the persistent volume, the loadbalancer is started immediately
	reloadB, reloadErr := lb.triggerReload()
	if reloadErr != nil {
		// Only log this error as it is expected to fail upon every fresh start
		log.Printf("initial trigger reload error: %s", reloadErr)
	}
	log.Printf("initial trigger reload: %s", string(reloadB))

	for {
		log.Print("begin reload haproxy")
		if err := lb.reloadHap(); err != nil {
			log.Printf("failed while reloading haproxy: %s", err)
			time.Sleep(time.Second)
			continue
		}
		log.Print("finish reload haproxy")

		waitForReload(sigC, defaultReloadWait)
	}
}

func maybeGenCert(autoCert bool, workDir string) error {
	if !autoCert {
		log.Print("not generating self-signed cert")
		return nil
	}

	pem := filepath.Join(workDir, autocertpemFile)
	crt := filepath.Join(workDir, autocertcrtFile)
	key := filepath.Join(workDir, autocertkeyFile)

	if ok, err := filesExist(pem, crt, key); err != nil {
		return err
	} else if ok {
		log.Print("self-signed cert already generated")
		return nil
	}

	log.Print("generating self-signed cert")
	return genCert(pem, crt, key)
}

func filesExist(files ...string) (bool, error) {
	for _, f := range files {
		if ok, err := fileExists(f); err != nil {
			return false, err
		} else if !ok {
			return false, nil
		}
	}
	return true, nil
}

func mustGetPath(f string) string {
	path, err := absPath(f)
	if err != nil {
		log.Fatalf("failed path %s: %s", f, err)
	}
	return path
}

func absPath(s string) (string, error) {
	maybeRelativePath, err := exec.LookPath(s)
	if err != nil {
		return "", err
	}
	return filepath.Abs(maybeRelativePath)
}

func (lb *lbState) safeTriggerReload() error {
	b, err := lb.addFirewallRules()
	if err != nil {
		return err
	}
	log.Printf("add firewall rules: %s", string(b))

	b, err = lb.saveHaproxyState()
	if err != nil {
		return err
	}
	log.Printf("save haproxy state: %s", string(b))

	b, err = lb.triggerReload()
	if err != nil {
		return err
	}
	log.Printf("trigger reload: %s", string(b))

	b, err = lb.removeFirewallRules()
	if err != nil {
		return err
	}
	log.Printf("remove firewall rules: %s", string(b))

	return nil
}

func (lb *lbState) reloadHap() error {
	// In this function we wait for the old haproxy pid to terminate and
	// the new one to start. This is behavior inherited from marathon-lb. It
	// is not clear why this is done, as haproxy itself takes care of the
	// reload.

	if err := lb.updateConfig(); err != nil {
		return err
	}

	pidExist, existErr := fileExists(lb.pidFile)
	if existErr != nil {
		return existErr
	}

	oldpid := -1
	if pidExist {
		// oldpidErr is instantiated outside to prevent shadowing with `:=`
		var oldpidErr error
		oldpid, oldpidErr = lb.getpid()
		if oldpidErr != nil {
			return oldpidErr
		}
	}
	log.Printf("old haproxy pid: %d", oldpid)

	if err := lb.safeTriggerReload(); err != nil {
		return err
	}

	newpid := -1
	for {
		// newpidErr is instantiated outside to prevent shadowing with `:=`
		var newpidErr error
		newpid, newpidErr = lb.getpid()
		if newpidErr != nil {
			return newpidErr
		}
		if newpid != oldpid {
			break
		}
		time.Sleep(defaultFastWait)
	}
	log.Printf("new haproxy pid: %d", newpid)

	if pidExist {
		// We wait for the old pid to be inactive and the new pid to be active
		// because the old haproxy will remain active until all connections
		// terminate.
		if err := pidwait(oldpid, false); err != nil {
			return err
		}
	}
	log.Printf("old pid %d is inactive", oldpid)

	if err := pidwait(newpid, true); err != nil {
		return err
	}
	log.Printf("new pid %d is active", newpid)

	return nil
}

func (lb *lbState) triggerReload() ([]byte, error) {
	ok, existErr := fileExists(lb.pidFile)
	if existErr != nil {
		return nil, existErr
	}
	if !ok {
		log.Print("pid file does not exist, running new haproxy")
		return execRun(lb.haproxyCmd())
	}

	oldpid, oldpidErr := lb.getpid()
	if oldpidErr != nil {
		return nil, oldpidErr
	}

	return execRun(lb.haproxyCmd("-sf", strconv.Itoa(oldpid)))
}

// Wait until pid is active/inactive according to targetActivity
func pidwait(pid int, targetActivity bool) error {
	for {
		active, err := pidActive(pid)
		if err != nil {
			return err
		}
		if active == targetActivity {
			return nil
		}
		time.Sleep(defaultFastWait)
	}
}

// A PID is considered inactive if sending a signal fails. This may produce
// false negatives, but that needs to be checked.
func pidActive(pid int) (bool, error) {
	p, findErr := os.FindProcess(pid)
	if findErr != nil {
		return false, findErr
	}

	// Signal 0 is special in that it doesn't do anything, that allows us
	// to use it to check liveness.
	signalErr := p.Signal(syscall.Signal(0))
	return (signalErr == nil), nil
}

func (lb *lbState) getpid() (int, error) {
	b, readErr := ioutil.ReadFile(lb.pidFile)
	if readErr != nil {
		return -1, readErr
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

func (lb *lbState) haproxyCmd(extraArgs ...string) *exec.Cmd {
	// There used to be a bit of odd bash in here that closed fd 200. This
	// was presumably related to the flock on fd 200. Since that flock is
	// no longer here, we'll assume that closing fd 200 is also no longer
	// necessary.

	cmdStr := fmt.Sprintf("%s %s -D -p %s -f %s", lb.hapWrapper, lb.hapExe, lb.pidFile, lb.hapConfig)
	cmdSlice := append(strings.Split(cmdStr, " "), extraArgs...)
	return exec.Command(cmdSlice[0], cmdSlice[1:]...)
}

func (lb *lbState) saveHaproxyState() ([]byte, error) {
	// From HAProxy docs: "Since 1.6, we can dump server states into a flat
	// file right before performing the reload and let the new process know
	// where the states are stored. That way, the old and new processes owns
	// exactly the same server states (hence seamless)."
	//
	// XXX We should break down this shell command and run it in go. We might
	//   not even need socat

	hapSocket := fmt.Sprintf("%s/socket", lb.hapWorkDir)
	hapState := fmt.Sprintf("%s/global", lb.hapStateDir)
	cmdStr := fmt.Sprintf(`socat %s - <<< "show servers state" > %s`, hapSocket, hapState)
	return execRun(exec.Command(lb.shellExe, "-c", cmdStr))
}

func (lb *lbState) removeFirewallRules() ([]byte, error) {
	output := []byte{}
	for _, p := range lb.ports {
		b, err := removeFwRule(lb.iptablesExe, p)
		if err != nil {
			return nil, fmt.Errorf("%s%s", output, err)
		}
		output = append(output, b...)
	}
	return output, nil
}

// This keeps trying to delete the rules until the command fails. Presumably
// that is because there may be duplicate rules and this is to keep deleting
// rules until it fails, upon which it is assumed that it means there are
// no more rules to delete.
//
// We are changing the logic here to require at least one success.
func removeFwRule(iptables string, port int) ([]byte, error) {
	output := []byte{}
	succeeded := false
	cmdStr := fmt.Sprintf("%s -w -D INPUT -p tcp --dport %d --syn -j DROP", iptables, port)
	for {
		splitCmd := strings.Split(cmdStr, " ")
		b, err := exec.Command(splitCmd[0], splitCmd[1:]...).CombinedOutput()
		if err == nil {
			succeeded = true
			output = append(output, b...)
			continue
		}

		if _, ok := err.(*exec.ExitError); ok {
			finalOut := append(output, b...)
			if succeeded {
				return finalOut, nil
			}
			return nil, fmt.Errorf(string(finalOut))
		}
		return nil, fmt.Errorf("fail removeFwRule: %s%s", string(output), err)
	}
}

func (lb *lbState) addFirewallRules() ([]byte, error) {
	// Even if this fails part way through, it is ok to just fail and restart
	// since adding an iptables rule twice is ok and the deletion portion
	// will take care of duplicates.

	output := []byte{}
	for _, p := range lb.ports {
		b, err := addFwRule(lb.iptablesExe, p)
		if err != nil {
			return nil, fmt.Errorf("%s%s", output, err)
		}
		output = append(output, b...)
	}
	return output, nil
}

func addFwRule(iptables string, port int) ([]byte, error) {
	cmdStr := fmt.Sprintf("%s -w -I INPUT -p tcp --dport %d --syn -j DROP", iptables, port)
	splitCmd := strings.Split(cmdStr, " ")
	return execRun(exec.Command(splitCmd[0], splitCmd[1:]...))
}

func (lb *lbState) updateConfig() error {
	log.Printf("waiting for %s", lb.apisReadyURL)
	if err := lb.runPing(); err != nil {
		return err
	}

	hapTmp := fmt.Sprintf("%s.tmp", lb.hapConfig)
	hapPrev := fmt.Sprintf("%s.prev", lb.hapConfig)
	log.Printf("downloading %s to %s", lb.poolConfigURL, hapTmp)
	body, httpErr := httpGetTextBody(lb.poolConfigURL)
	if httpErr != nil {
		return httpErr
	}
	if err := writeFile(hapTmp, body, defaultFileMode); err != nil {
		return fmt.Errorf("could not write haproxy config: %s", err)
	}

	log.Printf("validating %s", hapTmp)
	res, checkErr := checkHaproxyConfig(lb.hapExe, hapTmp)
	if checkErr != nil {
		return checkErr
	}
	log.Printf("\n%s", string(res))

	log.Print("moving configs into place")
	if err := configCurToPrev(lb.hapConfig, hapPrev); err != nil {
		return err
	}
	if err := os.Rename(hapTmp, lb.hapConfig); err != nil {
		return err
	}

	log.Printf("%s updated", lb.hapConfig)
	return nil
}

func configCurToPrev(cur, prev string) error {
	if ok, err := fileExists(cur); err != nil {
		return err
	} else if !ok {
		log.Printf("%s did not exist", cur)
		return nil
	}

	if err := os.Rename(cur, prev); err != nil {
		return err
	}
	return nil
}

func checkHaproxyConfig(haproxy, path string) ([]byte, error) {
	return execRun(exec.Command(haproxy, "-f", path, "-c"))
}

func execRun(cmd *exec.Cmd) ([]byte, error) {
	b, err := cmd.CombinedOutput()
	if err == nil {
		return b, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return nil, fmt.Errorf("%s%s", string(b), err.Error())
	}
	return nil, err
}

func (lb *lbState) runPing() error {
	bodyB, err := httpGetTextBody(lb.apisReadyURL)
	if err != nil {
		return err
	}
	body := string(bodyB)
	if body == pongMessage {
		return nil
	}
	return fmt.Errorf("runPing expected %s got %s", pongMessage, body)
}

func httpGetTextBody(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return nil, fmt.Errorf("httpGetBody request err: %s", err)
	}

	req.Header.Set("Accept", "text/plain")
	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("httpGetBody get err: %s", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := fmt.Sprintf("httpGetBody read err: %s", err)
		return nil, fmt.Errorf(apisUtil.MsgWithClose(msg, resp.Body))
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return nil, fmt.Errorf("httpGetBody fail close body: %s", closeErr)
	}
	return body, nil
}

func waitForReload(sigC chan os.Signal, wait time.Duration) {
	var reloadC <-chan time.Time = make(chan time.Time)
	var tmpReloadC <-chan time.Time

	for {
		select {
		case <-sigC:
			log.Print("got new signal")
			if tmpReloadC == nil {
				tmpReloadC = time.After(wait)
				log.Print("setting new timer")
			}
		case <-reloadC:
			return
		}

		reloadC = tmpReloadC
	}
}

func maybeEnvToFile(dstFileDir, envKeyPrefix string) error {
	if err := os.MkdirAll(dstFileDir, defaultDirMode); err != nil {
		return fmt.Errorf("error mkdir env path: %s", err)
	}

	for _, envKV := range os.Environ() {
		splitKV := strings.SplitN(envKV, "=", 2)
		envKey := splitKV[0]
		envValStr := splitKV[1]

		if !strings.HasPrefix(envKey, envKeyPrefix) {
			continue
		}

		dstFile := path.Join(dstFileDir, envKey)
		if ok, err := fileExists(dstFile); err != nil {
			return err
		} else if ok {
			log.Printf("%s already exists", dstFile)
			continue
		}

		envValBytes := []byte(envValStr)
		log.Printf("writing environment variable %s to %s", envKey, dstFile)
		if err := writeFile(dstFile, envValBytes, defaultFileMode); err != nil {
			return fmt.Errorf("error writing env to file: %s", err)
		}
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func writeFile(path string, b []byte, fileMode os.FileMode) error {
	tmpfile := fmt.Sprintf("%s.tmp", path)
	if err := ioutil.WriteFile(tmpfile, b, fileMode); err != nil {
		return fmt.Errorf("error writing to %s: %s", tmpfile, err)
	}
	if err := os.Rename(tmpfile, path); err != nil {
		return fmt.Errorf("error renaming %s to %s: %s", tmpfile, path, err)
	}
	return nil
}

func genCert(pemFilePath, crtFilePath, keyFilePath string) error {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Country:            []string{"Neuland"},
			Organization:       []string{"edgelb"},
			OrganizationalUnit: []string{"edgelb"},
		},
		Issuer: pkix.Name{
			Country:            []string{"Neuland"},
			Organization:       []string{"DC/OS"},
			OrganizationalUnit: []string{"DC/OS Networking Team"},
			Locality:           []string{"Neuland"},
			Province:           []string{"Neuland"},
			StreetAddress:      []string{"A datacenter near you"},
			PostalCode:         []string{"11111"},
			SerialNumber:       "23",
			CommonName:         "23",
		},
		SignatureAlgorithm:    x509.SHA512WithRSA,
		PublicKeyAlgorithm:    x509.ECDSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, 10),
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		BasicConstraintsValid: true,
		IsCA:        true,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate key: %s", err)
	}
	pub := &priv.PublicKey
	rawCert, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)
	if err != nil {
		return fmt.Errorf("create cert failed %s", err)
	}

	crtb := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rawCert,
	})
	keyb := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	log.Printf("writing auto pem to %s", pemFilePath)
	if err := writeFile(pemFilePath, append(crtb, keyb...), 0644); err != nil {
		return err
	}
	log.Printf("writing auto crt to %s", crtFilePath)
	if err := writeFile(crtFilePath, crtb, 0644); err != nil {
		return err
	}
	log.Printf("writing auto key to %s", keyFilePath)
	return writeFile(keyFilePath, keyb, 0644)
}
