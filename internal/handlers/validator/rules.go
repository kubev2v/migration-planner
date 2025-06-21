package validator

func NewAgentValidationRules() []ValidationRule {
	return []ValidationRule{
		{
			Tag:                    "sourceId",
			Rule:                   registerFn("sourceId", uuidValidator),
			ValidationErrorMessage: "source id should not be nil",
		},
		{
			Tag:                    "status",
			Rule:                   registerFn("status", agentStatusValidator),
			ValidationErrorMessage: "status should be one of [not-connected, waiting-for-credentials, error, gathering-initial-inventory, up-to-date, source-gone]",
		},
		{
			Tag:                    "statusInfo",
			Rule:                   registerAlias("statusInfo", "max=200"),
			ValidationErrorMessage: "status_info should not exceed 200 characters",
		},
		{
			Tag:                    "credentialUrl",
			Rule:                   registerAlias("credentialUrl", "url"),
			ValidationErrorMessage: "credentialUrl should be a valid url",
		},
		{
			Tag:                    "version",
			Rule:                   registerAlias("version", "max=20"),
			ValidationErrorMessage: "version should not exceed 20 characters",
		},
	}
}

func NewSourceValidationRules() []ValidationRule {
	return []ValidationRule{
		{
			Tag:                    "name",
			Rule:                   registerAlias("name", "source_name,min=1,max=100"),
			ValidationErrorMessage: "source name should have between 1 and 100 chars",
		},
		{
			Tag:                    "httpUrl",
			Rule:                   registerAlias("httpUrl", "url,startsnotwith=https"),
			ValidationErrorMessage: "http proxy url must be a valid url and not starting with https",
		},
		{
			Tag:                    "httpsUrl",
			Rule:                   registerAlias("httpsUrl", "url,startswith=https"),
			ValidationErrorMessage: "https proxy url must be a valid url and it should start with https",
		},
		{
			Tag:                    "noProxy",
			Rule:                   registerAlias("noProxy", "max=1000"),
			ValidationErrorMessage: "noProxy should have maximum 1000 characters",
		},
		{
			Tag:                    "sshPublicKey",
			Rule:                   registerAlias("sshPublicKey", "omitnil,ssh_key"),
			ValidationErrorMessage: "invalid ssh key",
		},
		{
			Tag:                    "certificateChain",
			Rule:                   registerAlias("certificateChain", "omitnil,certs"),
			ValidationErrorMessage: "invalid certificate chain",
		},
		{
			Tag:                    "proxy",
			Rule:                   registerAlias("proxy", "omitnil"),
			ValidationErrorMessage: "invalid proxy definition",
		},
		{
			Tag:                    "ssh_key",
			Rule:                   registerFn("ssh_key", sshKeyValidator),
			ValidationErrorMessage: "invalid ssh key",
		},
		{
			Tag:                    "source_name",
			Rule:                   registerFn("source_name", nameValidator),
			ValidationErrorMessage: "source name contains invalid characters",
		},
		{
			Tag:                    "certs",
			Rule:                   registerFn("certs", certificateValidator),
			ValidationErrorMessage: "invalid certificate chain",
		},
	}
}
