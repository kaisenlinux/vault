package pki

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

func pathIssuerGenerateRoot(b *backend) *framework.Path {
	return buildPathGenerateRoot(b, "issuers/generate/root/"+framework.GenericNameRegex("exported"))
}

func pathRotateRoot(b *backend) *framework.Path {
	return buildPathGenerateRoot(b, "root/rotate/"+framework.GenericNameRegex("exported"))
}

func buildPathGenerateRoot(b *backend, pattern string) *framework.Path {
	ret := &framework.Path{
		Pattern: pattern,

		Operations: map[logical.Operation]framework.OperationHandler{
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.pathCAGenerateRoot,
				// Read more about why these flags are set in backend.go
				ForwardPerformanceStandby:   true,
				ForwardPerformanceSecondary: true,
			},
		},

		HelpSynopsis:    pathGenerateRootHelpSyn,
		HelpDescription: pathGenerateRootHelpDesc,
	}

	ret.Fields = addCACommonFields(map[string]*framework.FieldSchema{})
	ret.Fields = addCAKeyGenerationFields(ret.Fields)
	ret.Fields = addCAIssueFields(ret.Fields)
	return ret
}

func pathIssuerGenerateIntermediate(b *backend) *framework.Path {
	return buildPathGenerateIntermediate(b,
		"issuers/generate/intermediate/"+framework.GenericNameRegex("exported"))
}

func pathCrossSignIntermediate(b *backend) *framework.Path {
	return buildPathGenerateIntermediate(b, "intermediate/cross-sign")
}

func buildPathGenerateIntermediate(b *backend, pattern string) *framework.Path {
	ret := &framework.Path{
		Pattern: pattern,
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.pathGenerateIntermediate,
				// Read more about why these flags are set in backend.go
				ForwardPerformanceStandby:   true,
				ForwardPerformanceSecondary: true,
			},
		},

		HelpSynopsis:    pathGenerateIntermediateHelpSyn,
		HelpDescription: pathGenerateIntermediateHelpDesc,
	}

	ret.Fields = addCACommonFields(map[string]*framework.FieldSchema{})
	ret.Fields = addCAKeyGenerationFields(ret.Fields)
	ret.Fields["add_basic_constraints"] = &framework.FieldSchema{
		Type: framework.TypeBool,
		Description: `Whether to add a Basic Constraints
extension with CA: true. Only needed as a
workaround in some compatibility scenarios
with Active Directory Certificate Services.`,
	}

	// Signature bits isn't respected on intermediate generation, as this
	// only impacts the CSR's internal signature and doesn't impact the
	// signed certificate's bits (that's on the /sign-intermediate
	// endpoints). Remove it from the list of fields to avoid confusion.
	delete(ret.Fields, "signature_bits")

	return ret
}

func pathImportIssuer(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "issuers/import/(cert|bundle)",
		Fields: map[string]*framework.FieldSchema{
			"pem_bundle": {
				Type: framework.TypeString,
				Description: `PEM-format, concatenated unencrypted
secret-key (optional) and certificates.`,
			},
		},

		Operations: map[logical.Operation]framework.OperationHandler{
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.pathImportIssuers,
				// Read more about why these flags are set in backend.go
				ForwardPerformanceStandby:   true,
				ForwardPerformanceSecondary: true,
			},
		},

		HelpSynopsis:    pathImportIssuersHelpSyn,
		HelpDescription: pathImportIssuersHelpDesc,
	}
}

func (b *backend) pathImportIssuers(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	// Since we're planning on updating issuers here, grab the lock so we've
	// got a consistent view.
	b.issuersLock.Lock()
	defer b.issuersLock.Unlock()

	keysAllowed := strings.HasSuffix(req.Path, "bundle") || req.Path == "config/ca"

	if b.useLegacyBundleCaStorage() {
		return logical.ErrorResponse("Can not import issuers until migration has completed"), nil
	}

	var pemBundle string
	var certificate string
	rawPemBundle, bundleOk := data.GetOk("pem_bundle")
	rawCertificate, certOk := data.GetOk("certificate")
	if bundleOk {
		pemBundle = rawPemBundle.(string)
	}
	if certOk {
		certificate = rawCertificate.(string)
	}

	if len(pemBundle) == 0 && len(certificate) == 0 {
		return logical.ErrorResponse("'pem_bundle' and 'certificate' parameters were empty"), nil
	}
	if len(pemBundle) > 0 && len(certificate) > 0 {
		return logical.ErrorResponse("'pem_bundle' and 'certificate' parameters were both provided"), nil
	}
	if len(certificate) > 0 {
		keysAllowed = false
		pemBundle = certificate
	}
	if len(pemBundle) < 75 {
		// It is almost nearly impossible to store a complete certificate in
		// less than 75 bytes. It is definitely impossible to do so when PEM
		// encoding has been applied. Detect this and give a better warning
		// than "provided PEM block contained no data" in this case. This is
		// because the PEM headers contain 5*4 + 6 + 4 + 2 + 2 = 34 characters
		// minimum (five dashes, "BEGIN" + space + at least one character
		// identifier, "END" + space + at least one character identifier, and
		// a pair of new lines). That would leave 41 bytes for Base64 data,
		// meaning at most a 30-byte DER certificate.
		//
		// However, < 75 bytes is probably a good length for a file path so
		// suggest that is the case.
		return logical.ErrorResponse("provided data for import was too short; perhaps a path was passed to the API rather than the contents of a PEM file"), nil
	}

	var createdKeys []string
	var createdIssuers []string
	issuerKeyMap := make(map[string]string)

	// Rather than using certutil.ParsePEMBundle (which restricts the
	// construction of the PEM bundle), we manually parse the bundle instead.
	pemBytes := []byte(pemBundle)
	var pemBlock *pem.Block

	var issuers []string
	var keys []string

	// By decoding and re-encoding PEM blobs, we can pass strict PEM blobs
	// to the import functionality (importKeys, importIssuers). This allows
	// them to validate no duplicate issuers exist (and place greater
	// restrictions during parsing) but allows this code to accept OpenSSL
	// parsed chains (with full textual output between PEM entries).
	for len(bytes.TrimSpace(pemBytes)) > 0 {
		pemBlock, pemBytes = pem.Decode(pemBytes)
		if pemBlock == nil {
			return logical.ErrorResponse("provided PEM block contained no data"), nil
		}

		pemBlockString := string(pem.EncodeToMemory(pemBlock))

		switch pemBlock.Type {
		case "CERTIFICATE", "X509 CERTIFICATE":
			// Must be a certificate
			issuers = append(issuers, pemBlockString)
		case "CRL", "X509 CRL":
			// Ignore any CRL entries.
		case "EC PARAMS", "EC PARAMETERS":
			// Ignore any EC parameter entries. This is an optional block
			// that some implementations send, to ensure some semblance of
			// compatibility with weird curves. Go doesn't support custom
			// curves and 99% of software doesn't either, so discard them
			// without parsing them.
		default:
			// Otherwise, treat them as keys.
			keys = append(keys, pemBlockString)
		}
	}

	if len(keys) > 0 && !keysAllowed {
		return logical.ErrorResponse("private keys found in the PEM bundle but not allowed by the path; use /issuers/import/bundle"), nil
	}

	for keyIndex, keyPem := range keys {
		// Handle import of private key.
		key, existing, err := importKeyFromBytes(ctx, b, req.Storage, keyPem, "")
		if err != nil {
			return logical.ErrorResponse(fmt.Sprintf("Error parsing key %v: %v", keyIndex, err)), nil
		}

		if !existing {
			createdKeys = append(createdKeys, key.ID.String())
		}
	}

	for certIndex, certPem := range issuers {
		cert, existing, err := importIssuer(ctx, b, req.Storage, certPem, "")
		if err != nil {
			return logical.ErrorResponse(fmt.Sprintf("Error parsing issuer %v: %v\n%v", certIndex, err, certPem)), nil
		}

		issuerKeyMap[cert.ID.String()] = cert.KeyID.String()
		if !existing {
			createdIssuers = append(createdIssuers, cert.ID.String())
		}
	}

	response := &logical.Response{
		Data: map[string]interface{}{
			"mapping":          issuerKeyMap,
			"imported_keys":    createdKeys,
			"imported_issuers": createdIssuers,
		},
	}

	if len(createdIssuers) > 0 {
		err := b.crlBuilder.rebuild(ctx, b, req, true)
		if err != nil {
			return nil, err
		}
	}

	// While we're here, check if we should warn about a bad default key. We
	// do this unconditionally if the issuer or key was modified, so the admin
	// is always warned. But if unrelated key material was imported, we do
	// not warn.
	config, err := getIssuersConfig(ctx, req.Storage)
	if err == nil && len(config.DefaultIssuerId) > 0 {
		// We can use the mapping above to check the issuer mapping.
		if keyId, ok := issuerKeyMap[string(config.DefaultIssuerId)]; ok && len(keyId) == 0 {
			msg := "The default issuer has no key associated with it. Some operations like issuing certificates and signing CRLs will be unavailable with the requested default issuer until a key is imported or the default issuer is changed."
			response.AddWarning(msg)
			b.Logger().Error(msg)
		}

		// If we imported multiple issuers with keys (or matched existing
		// keys), and we set one of those as a default, warn the end-user we
		// might have selected the wrong one.
		if len(createdIssuers) > 1 {
			numCreatedIssuersWithKeys := 0
			defaultIssuerWasCreated := false
			for _, issuerId := range createdIssuers {
				if keyId, ok := issuerKeyMap[issuerId]; ok && len(keyId) != 0 {
					numCreatedIssuersWithKeys++
				}

				if config.DefaultIssuerId.String() == issuerId {
					defaultIssuerWasCreated = true
				}
			}

			if numCreatedIssuersWithKeys > 1 && defaultIssuerWasCreated {
				msg := "The imported bundle contained multiple certs matching keys, " +
					"the default issuer that was selected should be verified and manually changed if incorrect."
				response.AddWarning(msg)
				b.Logger().Error(msg)
			}
		}
	}

	return response, nil
}

const (
	pathImportIssuersHelpSyn  = `Import the specified issuing certificates.`
	pathImportIssuersHelpDesc = `
This endpoint allows importing the specified issuer certificates.

:type is either the literal value "cert", to only allow importing
certificates, else "bundle" to allow importing keys as well as
certificates.

Depending on the value of :type, the pem_bundle request parameter can
either take PEM-formatted certificates, and, if :type="bundle", unencrypted
secret-keys.
`
)
