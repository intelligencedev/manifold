from supertonic_sidecar.text import split_sentences


def test_groups_short_sentences_into_one_chunk():
    text = "Hello there. How are you? I am fine."
    assert split_sentences(text, max_len=300) == ["Hello there. How are you? I am fine."]


def test_splits_when_exceeding_max_len():
    text = "One sentence here. Two sentence here. Three sentence here."
    chunks = split_sentences(text, max_len=25)
    assert len(chunks) == 3
    assert all(len(c) <= 25 for c in chunks)
    assert chunks == ["One sentence here.", "Two sentence here.", "Three sentence here."]


def test_does_not_split_on_common_abbreviations():
    text = "Dr. Smith arrived. He was late."
    chunks = split_sentences(text, max_len=300)
    assert chunks == ["Dr. Smith arrived. He was late."]
    # An abbreviation must not become its own chunk even under tight limits.
    tight = split_sentences(text, max_len=20)
    assert "Dr." not in tight
    assert tight == ["Dr. Smith arrived.", "He was late."]


def test_empty_or_whitespace_returns_empty_list():
    assert split_sentences("", max_len=300) == []
    assert split_sentences("   \n  ", max_len=300) == []


def test_splits_paragraphs():
    text = "First para.\n\nSecond para."
    assert split_sentences(text, max_len=300) == ["First para.", "Second para."]
