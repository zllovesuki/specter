package waf

// The engine configuration stays coupled to Coraza. These overrides adapt its
// recommended configuration to a general HTTP tunnel, rather than imposing a
// web application's methods, media types, or upload-size policy.
const engineProfile = `
SecAuditEngine Off
SecResponseBodyAccess Off
SecUploadKeepFiles Off
SecRequestBodyLimitAction ProcessPartial

# A prefix of a valid JSON document is usually not itself valid JSON. Keep the
# parser-error guard except for this specific JSON truncation error. Resource
# limits, multipart parser errors, and other parsing failures still reject.
SecRuleRemoveById 200002
SecRule INBOUND_DATA_ERROR "@eq 1" "id:10001,phase:2,t:none,pass,nolog,chain"
    SecRule REQBODY_PROCESSOR "@streq JSON" "t:none,chain"
        SecRule REQBODY_PROCESSOR_ERROR_MSG "@streq invalid JSON" "t:none,ctl:ruleRemoveById=200002"
SecRule REQBODY_ERROR "!@eq 0" "id:200002,phase:2,t:none,deny,status:400,nolog"
`

// CRS's default method/content-type allowlists describe a conventional website.
// Specter also carries WebDAV, APIs, and binary/streaming protocols. Their URL
// and header checks remain enabled, with body inspection limited to formats
// handled by the request adapter.
const tunnelProfile = `
SecRuleRemoveById 911100 920420
`

func ruleCategory(tags []string) string {
	for _, tag := range tags {
		switch tag {
		case "attack-sqli":
			return "SQL injection"
		case "attack-xss":
			return "Cross-site scripting"
		case "attack-rce":
			return "Command injection"
		case "attack-lfi":
			return "Local file access"
		case "attack-rfi":
			return "Remote file access"
		case "attack-ssrf":
			return "Server-side request forgery"
		case "attack-protocol":
			return "HTTP request format"
		case "attack-injection-php":
			return "PHP injection"
		case "attack-injection-java":
			return "Java injection"
		case "attack-session-fixation":
			return "Session fixation"
		case "attack-reputation-scanner":
			return "Known scanner"
		}
	}
	return "Web attack pattern"
}
