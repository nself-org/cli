package verify

// Purpose: one canonical form of a user's nullable identity fields.
// Inputs: email, phone, display name (nil means SQL NULL). Outputs: a comparable string.
// Constraints: NULL and empty text stay distinct for email and phone, because hasura-auth
// keeps email unique; a NULL display name equals "" because the column may be NOT NULL.
func Identity(email, phone, display *string) string {
	field := func(p *string) string {
		if p == nil {
			return "N"
		}
		return "V" + *p
	}
	name := ""
	if display != nil {
		name = *display
	}
	return field(email) + "\x00" + field(phone) + "\x00" + name
}
