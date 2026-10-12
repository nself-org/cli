package hasurameta

// Purpose: the fixed error texts this package may show the user. Each is constant text with no
// input in it; importer.publicError replaces any other source error with a generic one.
import "github.com/nself-org/cli/internal/importer"

const (
	msgURLInvalid       = "source Hasura URL invalid"
	msgURLHTTP          = "source Hasura URL must use https unless it is loopback"
	msgRequestInvalid   = "source Hasura request invalid"
	msgUnreachable      = "source Hasura unreachable"
	msgResponse         = "source Hasura response unreadable"
	msgRefusedSecret    = "source Hasura refused the admin secret"
	msgRequestFailed    = "source Hasura request failed"
	msgVersion          = "source Hasura version unreadable"
	msgMetadata         = "source Hasura metadata unreadable"
	msgMetadataSource   = "source Hasura metadata has an invalid source"
	msgMetadataTable    = "source Hasura metadata has an invalid table"
	msgMetadataEncode   = "source Hasura metadata not encodable"
	msgMetadataFunction = "source Hasura metadata has an invalid function"
	msgNoDefault        = "source Hasura metadata has no database source named default"
	msgMetadataMember   = "metadata member write failed"
	msgBundleIntegrity  = "extracted bundle failed integrity checks"
	msgBundleCreate     = "bundle creation failed"
	msgMemberCopy       = "bundle member copy failed"
	msgManifest         = "bundle manifest write failed"
)

func init() {
	importer.AllowMessages(msgURLInvalid, msgURLHTTP, msgRequestInvalid, msgUnreachable, msgResponse, msgRefusedSecret, msgRequestFailed, msgVersion, msgMetadata, msgMetadataSource, msgMetadataTable, msgMetadataEncode, msgMetadataFunction, msgNoDefault, msgMetadataMember, msgBundleIntegrity, msgBundleCreate, msgMemberCopy, msgManifest)
}
