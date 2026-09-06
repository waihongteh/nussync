package papers

// Venue detection and ranking.
//
// Search, the digest and recommendations all mix arXiv preprints with papers
// that actually went through peer review. A preprint and its NeurIPS camera
// -ready look identical in the raw metadata, so this file works out what a
// paper's venue really is and turns that into a score:
//
//	tier 2 — a venue on the user's PaperTopVenues list ("NeurIPS 2025")
//	tier 1 — any other published venue, and workshop papers at top venues
//	tier 0 — preprint / unknown
//
// Evidence, best first: Semantic Scholar's venue / publicationVenue / journal
// name, the arXiv <arxiv:journal_ref>, the arXiv <arxiv:comment> ("Accepted at
// ACL 2025", "To appear in TMLR", "camera-ready"), and — weakest — the mere
// presence of a non-arXiv DOI.

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// VenueInfo is the outcome of venue detection for one paper.
type VenueInfo struct {
	Tier      int    // 2 top venue, 1 published, 0 preprint/unknown
	Short     string // display label, e.g. "NeurIPS 2025" or "preprint"
	Published bool   // Tier >= 1
}

// VenuePrefs are the user's ranking settings (Settings.PaperTopVenues and
// Settings.PaperPreferPublished).
type VenuePrefs struct {
	TopVenues       []string
	PreferPublished bool
}

// DefaultTopVenues is the shipped PaperTopVenues list: the canonical short
// names this package knows how to recognise.
func DefaultTopVenues() []string {
	return []string{
		"NeurIPS", "ICML", "ICLR", "ACL", "EMNLP", "NAACL", "EACL", "COLING",
		"AAAI", "IJCAI", "COLM", "TACL", "JMLR", "TMLR", "CVPR", "ICCV",
		"ECCV", "KDD", "WWW", "SIGIR", "USENIX Security", "IEEE S&P", "CCS",
		"NDSS", "ICSE", "FSE",
	}
}

// DefaultVenuePrefs is the zero-config ranking configuration.
func DefaultVenuePrefs() VenuePrefs {
	return VenuePrefs{TopVenues: DefaultTopVenues(), PreferPublished: true}
}

// Normalised returns prefs with a usable top-venue list.
func (v VenuePrefs) Normalised() VenuePrefs {
	if len(v.TopVenues) == 0 {
		v.TopVenues = DefaultTopVenues()
	}
	return v
}

// isTop reports whether canon is on the user's top-venue list (case- and
// space-insensitive, so "usenix security" matches "USENIX Security").
func (v VenuePrefs) isTop(canon string) bool {
	if canon == "" {
		return false
	}
	want := foldName(canon)
	for _, t := range v.TopVenues {
		if foldName(t) == want {
			return true
		}
	}
	return false
}

func foldName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// ------------------------------------------------------------- venue names

// venueAlias maps a canonical short name onto the spellings seen in the wild.
type venueAlias struct {
	canon string
	// risky aliases are short words that occur in ordinary prose ("acl",
	// "www", "fse"), so they only count when a year sits next to them or the
	// whole candidate string is just the venue.
	risky bool
	re    *regexp.Regexp
}

func alias(canon string, risky bool, pattern string) venueAlias {
	return venueAlias{canon: canon, risky: risky,
		re: regexp.MustCompile(`(?i)\b(?:` + pattern + `)\b`)}
}

// venueAliases is scanned in order, so longer/less ambiguous names come first.
var venueAliases = []venueAlias{
	alias("NeurIPS", false, `neurips|nips|(?:advances in )?neural information processing systems`),
	alias("ICML", false, `icml|international conference on machine learning`),
	alias("ICLR", false, `iclr|international conference on learning representations`),
	alias("JMLR", false, `jmlr|journal of machine learning research`),
	alias("TMLR", false, `tmlr|transactions on machine learning research`),
	alias("TACL", false, `tacl|transactions of the association for computational linguistics`),
	alias("EMNLP", false, `emnlp|empirical methods in natural language processing`),
	alias("NAACL", false, `naacl|north american chapter of the association for computational linguistics`),
	alias("EACL", false, `eacl|european chapter of the association for computational linguistics`),
	alias("COLING", false, `coling|international conference on computational linguistics`),
	alias("COLM", false, `colm|conference on language modeling`),
	alias("AAAI", false, `aaai|association for the advancement of artificial intelligence`),
	alias("IJCAI", false, `ijcai|international joint conference on artificial intelligence`),
	alias("CVPR", false, `cvpr|computer vision and pattern recognition`),
	alias("ICCV", false, `iccv|international conference on computer vision`),
	alias("ECCV", false, `eccv|european conference on computer vision`),
	alias("KDD", false, `kdd|knowledge discovery and data mining`),
	alias("SIGIR", false, `sigir|special interest group on information retrieval`),
	alias("USENIX Security", false, `usenix security|usenix sec`),
	alias("IEEE S&P", false, `ieee s&p|ieee symposium on security and privacy|symposium on security and privacy|oakland`),
	alias("NDSS", false, `ndss|network and distributed system security`),
	alias("ICSE", false, `icse|international conference on software engineering`),
	alias("CCS", true, `ccs|conference on computer and communications security`),
	alias("ACL", true, `acl|annual meeting of the association for computational linguistics`),
	alias("WWW", true, `www|the web conference|world wide web conference`),
	alias("FSE", true, `fse|foundations of software engineering`),
}

var (
	yearRe     = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	urlRe      = regexp.MustCompile(`(?i)https?://\S+`)
	preprintRe = regexp.MustCompile(`(?i)^(arxiv|arxiv preprint|corr|abs/.*|preprint|ssrn|biorxiv|openreview)\b`)
	// commentLead pulls the venue out of "Accepted at ICLR 2026 (spotlight)".
	commentLead = regexp.MustCompile(`(?i)\b(?:accepted\s+(?:as\s+\S+\s+)?(?:at|to|by|in|for)|to\s+appear\s+(?:in|at)|published\s+(?:in|at|as)|camera[-\s]?ready\s+(?:version\s+)?(?:for|at|of|in)?|appears?\s+in|in\s+proceedings\s+of|proceedings\s+of|presented\s+at)\s+(.{2,90})`)
	workshopRe  = regexp.MustCompile(`(?i)\bworkshop\b`)
)

// IsPreprintVenue reports whether a venue string is just a preprint marker
// ("arXiv cs.LG", "CoRR", ""), i.e. no evidence of publication.
func IsPreprintVenue(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || preprintRe.MatchString(s)
}

// DetectVenue works out the venue tier from every piece of evidence a paper
// carries. venue is the S2 venue / journal name or the arXiv journal_ref,
// comment is the arXiv <arxiv:comment>, doi the paper's DOI and year its
// publication year (used only to date a venue that names none).
func DetectVenue(venue, comment, doi string, year int, prefs VenuePrefs) VenueInfo {
	prefs = prefs.Normalised()
	comment = urlRe.ReplaceAllString(collapseSpace(comment), " ")
	venue = collapseSpace(venue)

	// Candidates, strongest first. A comment lead-in ("Accepted at …") is
	// stronger than the raw comment, which may be "5 pages, 3 figures".
	var cands []string
	if !IsPreprintVenue(venue) {
		cands = append(cands, venue)
	}
	lead := ""
	if m := commentLead.FindStringSubmatch(comment); m != nil {
		lead = trimClause(m[1])
		if lead != "" {
			cands = append(cands, lead)
		}
	}
	if comment != "" {
		cands = append(cands, comment)
	}

	for _, c := range cands {
		canon, ok := matchCanonical(c)
		if !ok {
			continue
		}
		info := VenueInfo{Tier: 1, Published: true}
		if prefs.isTop(canon) {
			info.Tier = 2
		}
		short := canon
		if y := venueYear(c, year); y != "" {
			short += " " + y
		}
		if workshopRe.MatchString(c) {
			// A workshop at a top venue is not the top venue.
			info.Tier = 1
			short += " Workshop"
		}
		info.Short = short
		return info
	}

	// A venue we do not recognise by name is still a publication.
	for _, c := range []string{venue, lead} {
		if strings.TrimSpace(c) == "" || IsPreprintVenue(c) {
			continue
		}
		return VenueInfo{Tier: 1, Short: shortenVenue(c), Published: true}
	}

	// Weakest evidence: a DOI that is not arXiv's own DataCite DOI.
	if isPublisherDOI(doi) {
		return VenueInfo{Tier: 1, Short: "published", Published: true}
	}
	return VenueInfo{Tier: 0, Short: "preprint"}
}

// matchCanonical returns the canonical venue name mentioned in s.
func matchCanonical(s string) (string, bool) {
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	for _, a := range venueAliases {
		loc := a.re.FindStringIndex(s)
		if loc == nil {
			continue
		}
		if a.risky && !riskyOK(s, loc) {
			continue
		}
		return a.canon, true
	}
	return "", false
}

// riskyOK gates the short, prose-like aliases: they only count when the whole
// candidate is essentially the venue, or a year sits right next to the match.
func riskyOK(s string, loc []int) bool {
	if len(strings.Fields(s)) <= 4 {
		return true
	}
	lo := loc[0] - 12
	if lo < 0 {
		lo = 0
	}
	hi := loc[1] + 12
	if hi > len(s) {
		hi = len(s)
	}
	return yearRe.MatchString(s[lo:hi])
}

// venueYear picks the year named in the venue string, falling back to the
// paper's own year. Returns "" when neither is usable.
func venueYear(s string, fallback int) string {
	if m := yearRe.FindString(s); m != "" {
		return m
	}
	if fallback >= 1900 && fallback <= time.Now().Year()+1 {
		return strconv.Itoa(fallback)
	}
	return ""
}

// trimClause cuts a captured phrase at the first sentence-ending punctuation.
func trimClause(s string) string {
	s = collapseSpace(s)
	// A comma normally starts the noise after the venue ("ACL 2025, 9 pages",
	// "WIPE-OUT 2026, the 2nd Workshop on ..."), so the clause ends there too.
	if i := strings.IndexAny(s, ".;!?,"); i >= 0 {
		s = s[:i]
	}
	// A trailing parenthetical ("(spotlight)") is noise, but keep the year.
	s = strings.TrimRight(s, " ,:-")
	// "Accepted at the 2026 …" — the article is not part of the venue name.
	for _, art := range []string{"the ", "The "} {
		s = strings.TrimPrefix(s, art)
	}
	return strings.TrimSpace(s)
}

// shortenVenue caps an unrecognised venue name for the badge.
func shortenVenue(s string) string {
	s = collapseSpace(s)
	if len(s) > 34 {
		s = trimBytes(s, 34) + "…"
	}
	return s
}

// isPublisherDOI reports whether a DOI points at a real publisher rather than
// arXiv's own 10.48550 prefix.
func isPublisherDOI(doi string) bool {
	doi = strings.ToLower(strings.TrimSpace(doi))
	if doi == "" {
		return false
	}
	return !strings.HasPrefix(doi, "10.48550")
}

// -------------------------------------------------------------- annotation

// AnnotateVenue fills VenueTier/VenueShort/Published on one paper. When the
// venue was only discoverable from the arXiv comment the detected short name
// is also written into Venue, so it survives a round-trip through the library
// table (which stores Venue but not the derived fields).
func AnnotateVenue(p *Paper, prefs VenuePrefs) {
	if p == nil {
		return
	}
	info := DetectVenue(p.Venue, p.Comment, p.DOI, p.Year, prefs)
	p.VenueTier = info.Tier
	p.VenueShort = info.Short
	p.Published = info.Published
	if info.Tier > 0 && IsPreprintVenue(p.Venue) && info.Short != "published" {
		p.Venue = info.Short
	}
}

// AnnotateVenues annotates a slice in place and returns it.
func AnnotateVenues(ps []Paper, prefs VenuePrefs) []Paper {
	for i := range ps {
		AnnotateVenue(&ps[i], prefs)
	}
	return ps
}

// FilterPublished keeps only papers with a detected venue (tier >= 1).
func FilterPublished(ps []Paper) []Paper {
	out := make([]Paper, 0, len(ps))
	for _, p := range ps {
		if p.VenueTier >= 1 {
			out = append(out, p)
		}
	}
	return out
}

// ------------------------------------------------------------------ score

// Score ranks a paper: venue tier (when the user prefers published work),
// citations on a log curve, and a recency bonus that decays over 12 months.
// Query relevance is not scored here — SortByScore keeps the source's own
// ordering as a small tie-break.
func Score(p Paper, prefs VenuePrefs) float64 {
	s := 0.0
	if prefs.PreferPublished {
		switch p.VenueTier {
		case 2:
			s += 100
		case 1:
			s += 40
		}
	}
	if p.CitationCount > 0 {
		s += math.Log(float64(p.CitationCount)+1) * 8
	}
	s += recencyBonus(p, time.Now())
	return s
}

// RecencyBonus is the maximum bonus for a paper published today.
const RecencyBonus = 15.0

func recencyBonus(p Paper, now time.Time) float64 {
	t, ok := parsePaperDate(p.PublishedAt)
	if !ok {
		return 0
	}
	months := now.Sub(t).Hours() / 24 / 30.44
	if months < 0 {
		months = 0
	}
	if months >= 12 {
		return 0
	}
	return RecencyBonus * (1 - months/12)
}

func parsePaperDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01", "2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// SourceRankBonus is the weight given to the source's own ordering, small
// enough to break ties without overriding the venue tier.
const SourceRankBonus = 5.0

// SortByScore annotates and reorders papers best-first. The incoming order
// (arXiv relevance, S2 relevance) survives as a small bonus, so equally scored
// papers keep the source's judgement of query relevance.
func SortByScore(ps []Paper, prefs VenuePrefs) []Paper {
	prefs = prefs.Normalised()
	AnnotateVenues(ps, prefs)
	n := len(ps)
	if n < 2 {
		return ps
	}
	scores := make(map[int]float64, n)
	idx := make([]int, n)
	for i := range ps {
		idx[i] = i
		scores[i] = Score(ps[i], prefs) + SourceRankBonus*float64(n-i)/float64(n)
	}
	sort.SliceStable(idx, func(a, b int) bool { return scores[idx[a]] > scores[idx[b]] })
	out := make([]Paper, 0, n)
	for _, i := range idx {
		out = append(out, ps[i])
	}
	copy(ps, out)
	return ps
}
