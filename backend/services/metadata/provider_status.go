package metadata

// TVDBConfigured reports whether TVDB is enabled with an API key. Consumers can
// use this to gate optional fallback providers without exposing credentials.
func (s *Service) TVDBConfigured() bool {
	return s != nil && s.client.isConfigured()
}
