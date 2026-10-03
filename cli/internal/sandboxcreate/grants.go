package sandboxcreate

import (
	"fmt"
	"slices"
	"strings"

	apiclientgen "github.com/discobox-ai/discobox/api/gen"
	apimodel "github.com/discobox-ai/discobox/api/model"
)

// ParseGrants reads --grant values into the grants a create gives the new
// discobox. A value is SECRET[@HOST]:ENV_VAR=USE, or ID[@HOST]=USE for a
// well-known credential, whose ID carries its secret and variable; the first
// has a ":" before the "=" and the second does not. HOST may be several,
// joined by commas, for a credential sent to more than one site (ADR
// 26-10-02-393). Values naming the same
// credential, host, and variable are one grant with several uses, in the order
// given. A secret is left as written, for the caller to resolve to an ID.
func ParseGrants(values []string) ([]apimodel.SandboxGrant, error) {
	var grants []apimodel.SandboxGrant
	index := map[string]int{}
	for _, value := range values {
		spec, use, ok := strings.Cut(value, "=")
		use = strings.TrimSpace(use)
		target, envVar, named := strings.Cut(spec, ":")
		credential, host, _ := strings.Cut(target, "@")
		credential, host, envVar = strings.TrimSpace(credential), strings.TrimSpace(host), strings.TrimSpace(envVar)
		if !ok || credential == "" || use == "" || (named && envVar == "") {
			return nil, fmt.Errorf("--grant %q: want SECRET[@HOST]:ENV_VAR=USE, such as github@github.com:GH_TOKEN=\"push a branch to org/repo\", or ID[@HOST]=USE for a well-known credential, such as com.github.api=\"push a branch to org/repo\"", value)
		}
		// Keyed on the hosts as a set, as the server reads them: the same
		// hosts in another order or spacing are the same grant.
		hosts := grantHosts(host)
		key := credential + "@" + strings.Join(slices.Sorted(slices.Values(hosts)), ",") + ":" + envVar
		i, seen := index[key]
		if !seen {
			var grant apimodel.SandboxGrant
			if named {
				grant.SetSecretId(apiclientgen.NewOptString(credential))
				grant.SetEnvVar(apiclientgen.NewOptString(envVar))
			} else {
				grant.SetWellKnownId(apiclientgen.NewOptString(credential))
			}
			// None named leaves the list out, which takes the secret's host,
			// or the well-known credential's.
			grant.SetHosts(hosts)
			grants = append(grants, grant)
			i = len(grants) - 1
			index[key] = i
		}
		grants[i].Uses = append(grants[i].Uses, apimodel.SecretUse{Description: use})
	}
	return grants, nil
}

// grantHosts are the comma-joined hosts of a --grant value, trimmed and
// lowercased, without empties or repeats, in the order given.
func grantHosts(value string) []string {
	var hosts []string
	for _, host := range strings.Split(value, ",") {
		if host = strings.ToLower(strings.TrimSpace(host)); host != "" && !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	return hosts
}
