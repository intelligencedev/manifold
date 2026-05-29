package textsplitters

// RecursiveConfig layers multiple strategies top-down.
type RecursiveConfig struct {
	Markdown   MarkdownConfig
	Paragraphs BoundaryConfig
	Sentences  BoundaryConfig
	Fallback   FixedConfig
}

type recursiveSplitter struct{ cfg RecursiveConfig }

func newRecursiveSplitter(cfg RecursiveConfig) (Splitter, error) {
	return &recursiveSplitter{cfg: cfg}, nil
}

func (r *recursiveSplitter) Split(text string) []string {
	// stage 1: markdown sections
	md, err := newMarkdownSplitter(r.cfg.Markdown)
	if err != nil {
		return nil
	}
	mdChunks := md.Split(text)
	if len(mdChunks) == 0 {
		mdChunks = []string{text}
	}
	var out []string
	for _, sec := range mdChunks {
		if len(sec) == 0 {
			continue
		}
		// stage 2: paragraphs
		p, err := newParagraphSplitter(r.cfg.Paragraphs)
		if err != nil {
			out = append(out, sec)
			continue
		}
		pChunks := p.Split(sec)
		if len(pChunks) == 0 {
			pChunks = []string{sec}
		}
		for _, pc := range pChunks {
			// stage 3: sentences
			s, err := newSentenceSplitter(r.cfg.Sentences)
			if err != nil {
				out = append(out, pc)
				continue
			}
			sChunks := s.Split(pc)
			if len(sChunks) == 0 {
				sChunks = []string{pc}
			}
			for _, sc := range sChunks {
				// final: ensure max via fixed if needed
				if r.cfg.Fallback.Size > 0 {
					fx, err := newFixedSplitter(r.cfg.Fallback)
					if err != nil {
						out = append(out, sc)
						continue
					}
					out = append(out, fx.Split(sc)...)
				} else {
					out = append(out, sc)
				}
			}
		}
	}
	return out
}
