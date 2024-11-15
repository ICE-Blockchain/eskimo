// SPDX-License-Identifier: ice License 1.0

package scraper

import (
	"bytes"
	"context"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

func newCMCVerifier(sc webScraper, countries []string) *cmcVerifierImpl {
	return &cmcVerifierImpl{
		Scraper:   sc,
		Countries: countries,
	}
}

func (c *cmcVerifierImpl) VerifyPost(ctx context.Context, meta *Metadata) (username string, err error) {
	if !validateProfileURL(meta.PostURL) {
		return "", errors.Wrapf(ErrInvalidURL, "invalid url: %v", meta.PostURL)
	}
	postURL := normalizeProfileURL(meta.PostURL)
	oe, err := c.Scrape(ctx, postURL)
	if err != nil {
		return "", errors.Wrapf(err, "can't scrape: %v", postURL)
	}

	return "", errors.Wrapf(verifyFollowing(oe.Content, meta), "can't verify following: %v", postURL)
}

func verifyFollowing(html []byte, meta *Metadata) (err error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return multierror.Append(ErrInvalidPageContent, err)
	}
	found := false
	doc.Find("div.name div.nickName").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		found = strings.EqualFold(s.Find("span").Text(), meta.ExpectedPostText)

		return !found
	})
	if !found {
		return errors.Wrapf(ErrTextNotFound, "text %v not found", meta.ExpectedPostText)
	}

	return nil
}

func validateProfileURL(url string) bool {
	return strings.HasPrefix(url, "https://coinmarketcap.com/community/profile")
}

func normalizeProfileURL(url string) string {
	res := strings.TrimSuffix(url, "/")
	if !strings.HasSuffix(res, "following") {
		res += "/following"
	}

	return res
}

func (c *cmcVerifierImpl) Scrape(ctx context.Context, target string) (result *webScraperResult, err error) { //nolint:funlen // .
	const (
		trueVal  = "true"
		falseVal = "false"
	)

	for _, country := range c.countries() {
		if result, err = c.Scraper.Scrape(ctx, target,
			webScraperOptions{
				Retry: twitterRetryFn,
				Options: func(options map[string]string) map[string]string {
					options["country"] = country
					options["wait_for_css"] = ".user-card__container"
					options["auto_solve"] = trueVal
					options["block_resources"] = falseVal
					options["load_iframes"] = trueVal
					options["load_shadowroots"] = falseVal

					return options
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			}); err == nil {
			break
		}
	}
	if err != nil {
		return nil, multierror.Append(ErrFetchFailed, err)
	}
	if result == nil {
		return nil, errors.Wrap(ErrFetchFailed, "no result")
	}

	switch result.Code {
	case http.StatusOK:
		return result, nil
	default:
		return nil, errors.Wrapf(ErrFetchFailed, "%q: unexpected status code: `%v`, response: `%v`", target, result.Code, string(result.Content))
	}
}

func (c *cmcVerifierImpl) countries() []string {
	countries := slices.Clone(c.Countries)
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(countries), func(ii, jj int) { //nolint:gosec // .
		countries[ii], countries[jj] = countries[jj], countries[ii]
	})

	return removeDuplicates(countries)
}
