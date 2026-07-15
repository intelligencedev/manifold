package lexminify

type phrase struct {
	from string
	to   string
}

// Longest phrases first so nested matches prefer longer forms.
var fillerPhrases = []phrase{
	{from: "as mentioned previously", to: ""},
	{from: "as previously mentioned", to: ""},
	{from: "it is important to note that", to: ""},
	{from: "it is worth noting that", to: ""},
	{from: "please be aware that", to: ""},
	{from: "for the purpose of", to: "for"},
	{from: "with regard to", to: "re"},
	{from: "with respect to", to: "re"},
	{from: "in the event that", to: "if"},
	{from: "in order to", to: "to"},
	{from: "due to the fact that", to: "because"},
	{from: "please note that", to: ""},
	{from: "it should be noted that", to: ""},
	{from: "at this point in time", to: "now"},
	{from: "in light of the fact that", to: "because"},
	{from: "for all intents and purposes", to: ""},
	{from: "as a matter of fact", to: ""},
	{from: "needless to say", to: ""},
	{from: "as you can see", to: ""},
	{from: "it is important to", to: ""},
	{from: "in terms of", to: "re"},
	{from: "a large number of", to: "many"},
	{from: "a number of", to: "some"},
	{from: "in addition to", to: "plus"},
	{from: "as well as", to: "and"},
	{from: "in the case of", to: "for"},
	{from: "on the other hand", to: "but"},
	{from: "in other words", to: ""},
	{from: "to the extent that", to: "if"},
	{from: "with the exception of", to: "except"},
	{from: "despite the fact that", to: "although"},
	{from: "in the near future", to: "soon"},
	{from: "at the present time", to: "now"},
	{from: "in order that", to: "so"},
	{from: "prior to", to: "before"},
	{from: "subsequent to", to: "after"},
	{from: "in accordance with", to: "per"},
	{from: "by means of", to: "via"},
	{from: "for the most part", to: "mostly"},
	{from: "in the majority of cases", to: "usually"},
	{from: "it is clear that", to: ""},
	{from: "keep in mind that", to: ""},
	{from: "bear in mind that", to: ""},
}

var abbrevPhrases = []phrase{
	{from: "approximately", to: "~"},
	{from: "for example", to: "e.g."},
	{from: "that is", to: "i.e."},
	{from: "and so on", to: "etc"},
}

func containsAnyPhrase(lower string) bool {
	for _, p := range fillerPhrases {
		if len(p.from) > 0 && indexOf(lower, p.from) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	n := len(sub)
	if n == 0 || n > len(s) {
		return -1
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == sub {
			return i
		}
	}
	return -1
}

// protectedWords MUST never be dropped by stopword or telegram stages.
var protectedWords = map[string]bool{
	"not":       true,
	"no":        true,
	"never":     true,
	"without":   true,
	"except":    true,
	"only":      true,
	"must":      true,
	"cannot":    true,
	"can't":     true,
	"don't":     true,
	"won't":     true,
	"shouldn't": true,
	"wouldn't":  true,
	"couldn't":  true,
	"needn't":   true,
	"nor":       true,
	"none":      true,
	"nothing":   true,
	"neither":   true,
	"unless":    true,
	"against":   true,
	"however":   true,
	"although":  true,
	"but":       true,
	"or":        true,
	"if":        true,
	"when":      true,
	"while":     true,
	"because":   true,
	"since":     true,
	"until":     true,
	"always":    true,
	"true":      true,
	"false":     true,
	"yes":       true,
	"null":      true,
	"nil":       true,
	"error":     true,
	"failed":    true,
	"success":   true,
}

// Careful stopword set for L2. Content-bearing short words are absent.
// Words with useful L3 abbreviations (with, without, about, because) are
// intentionally omitted so later stages can rewrite them.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true,
	"am": true, "is": true, "are": true, "was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "having": true, "do": true, "does": true, "did": true, "doing": true,
	"of": true, "at": true, "by": true, "for": true,
	"into": true, "through": true, "during": true, "before": true, "after": true, "above": true, "below": true,
	"to": true, "from": true, "up": true, "down": true, "in": true, "out": true, "on": true, "off": true, "over": true, "under": true,
	"again": true, "further": true, "then": true, "once": true,
	"here": true, "there": true, "all": true, "each": true, "few": true, "more": true, "most": true, "other": true, "some": true, "such": true,
	"than": true, "too": true, "very": true, "just": true, "also": true,
	"and": true,
	"as":  true, "so": true,
	"this": true, "that": true, "these": true, "those": true,
	"i": true, "me": true, "my": true, "we": true, "our": true, "you": true, "your": true,
	"he": true, "him": true, "his": true, "she": true, "her": true, "it": true, "its": true, "they": true, "them": true, "their": true,
	"what": true, "which": true, "who": true, "whom": true,
	"can": true, "will": true, "would": true, "should": true, "could": true, "may": true, "might": true,
	"shall": true,
	"own": true, "same": true,
}

var telegramDrop = map[string]bool{
	"please": true, "kindly": true, "actually": true, "basically": true, "really": true,
	"quite": true, "rather": true, "perhaps": true, "maybe": true, "somewhat": true,
	"various": true, "certain": true, "respective": true, "respectively": true,
	"currently": true, "generally": true, "typically": true, "often": true, "usually": true,
	"simply": true, "essentially": true, "literally": true, "obviously": true,
	"clearly": true, "certainly": true, "definitely": true, "probably": true,
	"seem": true, "seems": true, "appear": true, "appears": true,
	"regarding": true, "concerning": true, "towards": true, "toward": true,
}

var abbrevWords = map[string]string{
	"with":           "w/",
	"without":        "w/o",
	"approximately":  "~",
	"about":          "~",
	"versus":         "vs",
	"configuration":  "cfg",
	"application":    "app",
	"information":    "info",
	"repository":     "repo",
	"directory":      "dir",
	"function":       "fn",
	"parameter":      "param",
	"parameters":     "params",
	"argument":       "arg",
	"arguments":      "args",
	"message":        "msg",
	"messages":       "msgs",
	"request":        "req",
	"response":       "resp",
	"number":         "#",
	"because":        "bc",
	"between":        "btw",
	"example":        "eg",
	"examples":       "egs",
	"maximum":        "max",
	"minimum":        "min",
	"average":        "avg",
	"environment":    "env",
	"implementation": "impl",
	"documentation":  "docs",
	"dependency":     "dep",
	"dependencies":   "deps",
	"database":       "db",
	"service":        "svc",
	"services":       "svcs",
	"manager":        "mgr",
	"context":        "ctx",
	"errors":         "errs",
	"string":         "str",
	"boolean":        "bool",
	"integer":        "int",
	"object":         "obj",
	"value":          "val",
	"values":         "vals",
	"variable":       "var",
	"variables":      "vars",
	"pointer":        "ptr",
	"reference":      "ref",
	"references":     "refs",
	"package":        "pkg",
	"packages":       "pkgs",
	"available":      "avail",
	"unavailable":    "unavail",
	"synchronous":    "sync",
	"asynchronous":   "async",
	"initialize":     "init",
	"initialization": "init",
	"configure":      "cfg",
}
