package pagesnatcher

var Profiles = map[string]Profile{
	"framer":                  Framer,
	"framer-remove-watermark": FramerNoWaterMark,
	"webflow":                 Webflow,
}

var Framer Profile = Profile{
	Domains: []string{
		"framerusercontent.com",
		"events.framer.com",
		"framer.app",
		"framer.website",
	},
	ReplaceStrings: map[string]string{
		"../events.framer.com": "./events.framer.com",
		"../framerusercontent": "./framerusercontent",
		"./framer.com/m/":      "framer.com/m/",
	},
	QueryFilePaths: []string{"?lossless=1"},
}

var FramerNoWaterMark Profile = Profile{
	Domains: Framer.Domains,
	ReplaceStrings: map[string]string{
		"../events.framer.com":                "./events.framer.com",
		"../framerusercontent":                "./framerusercontent",
		`<div id="__framer-badge-container">`: `<div id="__framer-badge-container" style="display: none">`,
	},
	QueryFilePaths: Framer.QueryFilePaths,
}

var Webflow Profile = Profile{
	Domains: []string{
		"website-files.com",
	},
}
