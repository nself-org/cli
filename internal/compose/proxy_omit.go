package compose

// OmitNginx clears only the nginx service gate. ADR 0027: the Traefik
// plugin fragment serves 80/443; nginx/ and routes.json are still generated.
func (g *Generator) OmitNginx() *Generator {
	g.profile.Nginx = false
	return g
}
