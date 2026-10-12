// Package hasurameta reads a source Hasura's metadata and recomposes a portable bundle.
// Purpose: the metadata half shared by the nhost and hasura-cloud sources (EPIC D10/D11).
// Inputs: a Hasura base URL, an admin secret and the source bundle's table list.
// Outputs: hasura/metadata.json (only imported tables and functions, no connection strings,
// event triggers, remote schemas or actions) and the manifest counts.
// Constraints: the admin secret travels only in the request header and never in an error;
// plain http is accepted for loopback only.
package hasurameta
