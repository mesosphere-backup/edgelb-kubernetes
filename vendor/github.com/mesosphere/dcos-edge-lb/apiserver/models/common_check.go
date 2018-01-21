package models

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// All lowercase. Must start and end with alphanumeric. Internals can
	// also have dashes.
	nameRegex = `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`

	// This basically just checks that it begins and ends with alphanumeric.
	// The interior is validated by the nameRegex. It can also be empty,
	// which is not checked in here.
	namespaceRegex = `^[a-z0-9](.*[a-z0-9])?$`

	// No absolute paths and no subdirectories, so no `/`.
	// Disallow `.` so we don't have to check for `.` and `..`.
	secretFileRegex = `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`

	// No absolute paths and only a single subdirectory.
	// Disallow `.` so we don't have to check for `.` and `..`.
	// Must begin with a special envvar.
	certificateRegex = `(^\$AUTOCERT$|^(\$ENVFILE|\$SECRETS)/[a-zA-Z0-9_-]+$)`
)

// CheckConfigContainer validates a config container
func CheckConfigContainer(cfg *ConfigContainer) error {
	pnames := make(map[string]struct{})
	for _, pool := range cfg.Pools {
		name := pool.Name
		if _, exist := pnames[name]; exist {
			// We purposefully leave out the namespace when checking. This is
			// because we use the name without the namespace in the HTTP API
			// url path.
			return failCheck("duplicate pool.name: %s", name)
		}
		pnames[name] = struct{}{}
	}

	for _, pool := range cfg.Pools {
		if err := CheckPoolContainer(pool); err != nil {
			return err
		}
	}
	return nil
}

// CheckPoolContainer validates a pool container
func CheckPoolContainer(pool *PoolContainer) error {
	if pool.APIVersion == APIVersionV1 {
		if pool.V1 == nil {
			return failCheck("invalid pool container, missing v1 for pool %s", pool.Name)
		}
		return V1CheckPool(pool.V1)
	}
	if pool.V2 == nil {
		return failCheck("invalid pool container, missing v2 for pool %s", pool.Name)
	}
	return V2CheckPool(pool.V2)
}

// Common utils for checks
func checkNamespace(ns string) error {
	if ns == "" {
		return nil
	}

	re, err := regexp.Compile(namespaceRegex)
	if err != nil {
		return err
	}

	if !re.MatchString(ns) {
		return fmt.Errorf("%s invalid: did not match %s", ns, namespaceRegex)
	}

	for _, s := range strings.Split(ns, "/") {
		if err := checkName(s); err != nil {
			return err
		}
	}
	return nil
}

func checkName(name string) error {
	re, err := regexp.Compile(nameRegex)
	if err != nil {
		return err
	}

	if !re.MatchString(name) {
		return fmt.Errorf("%s invalid: did not match %s", name, nameRegex)
	}
	return nil
}

func checkPort(name string, port int32) error {
	if port < 0 || port > 65535 {
		return failCheck("invalid %s: %d", name, port)
	}
	return nil
}

func checkSecretFile(sfile string) error {
	re, err := regexp.Compile(secretFileRegex)
	if err != nil {
		return err
	}

	if !re.MatchString(sfile) {
		return failCheck("%s invalid: did not match %s", sfile, secretFileRegex)
	}
	return nil
}

func frontendBackendCrossCheck(referencedBackends, benames map[string]struct{}) error {
	// At this point we've already checked for empty string backend names
	// so we don't need to strip it out of benames

	extraRef, extraBe := diffStrSets(referencedBackends, benames)
	if len(extraRef) != 0 {
		return failCheck("frontends refer to nonexistent backends: %s", strings.Join(extraRef, ", "))
	}
	if len(extraBe) != 0 {
		return failCheck("frontends do not refer to these backends: %s", strings.Join(extraBe, ", "))
	}
	return nil
}

func diffStrSets(s1, s2 map[string]struct{}) (extras1, extras2 []string) {
	for k := range s1 {
		if _, ok := s2[k]; !ok {
			extras1 = append(extras1, k)
		}
	}
	for k := range s2 {
		if _, ok := s1[k]; !ok {
			extras2 = append(extras2, k)
		}
	}
	return extras1, extras2
}

func checkCertificate(cert string) error {
	re, err := regexp.Compile(certificateRegex)
	if err != nil {
		return err
	}

	if !re.MatchString(cert) {
		return failCheck("%s invalid: did not match %s", cert, certificateRegex)
	}
	return nil
}

func getPathEnding(s string) string {
	if s == "" {
		return ""
	}
	if s[len(s)-1] == '/' {
		return "/"
	}
	return ""
}

func failString(s string) error {
	return failCheck("%s cannot be empty or the empty string", s)
}

func failCheck(format string, a ...interface{}) error {
	return fmt.Errorf(format, a...)
}
