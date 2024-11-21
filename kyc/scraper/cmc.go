// SPDX-License-Identifier: ice License 1.0

package scraper

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

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
	if !validatePostURL(meta.PostURL) {
		return "", errors.Wrapf(ErrInvalidURL, "invalid url: %v", meta.PostURL)
	}
	oe, err := c.Scrape(ctx, meta.PostURL)
	if err != nil {
		return "", errors.Wrapf(err, "can't scrape: %v", meta.PostURL)
	}

	return "", errors.Wrapf(verifyPost(oe.Content), "can't verify cmc link: %v", meta.PostURL)
}

//nolint:funlen // .
func verifyPost(html []byte) (err error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return multierror.Append(ErrInvalidPageContent, err)
	}
	var (
		foundIceCoin    = false
		foundPairedCoin = false
	)
	doc.Find("#post-detail a.coin-link").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		const coinPrefix = "$"
		txt := s.Find("span.real-text").Text()
		txt, foundPrefix := strings.CutPrefix(txt, coinPrefix)
		fmt.Printf("txt: %v\n", txt)
		if !foundPrefix {
			return false
		}
		if foundIce := strings.EqualFold(txt, iceCoinName); foundIce {
			foundIceCoin = true
		} else {
			for _, coinName := range supportedCMCCoinNameList {
				if foundPaired := strings.EqualFold(txt, coinName); foundPaired {
					foundPairedCoin = true

					break
				}
			}
		}

		return !foundIceCoin || !foundPairedCoin
	})
	if !foundIceCoin || !foundPairedCoin {
		return errors.Wrap(ErrTextNotFound, "ice coin or paired coin not found")
	}

	return nil
}

func validatePostURL(url string) bool {
	return strings.HasPrefix(url, "https://coinmarketcap.com")
}

func (c *cmcVerifierImpl) Scrape(ctx context.Context, target string) (result *webScraperResult, err error) { //nolint:funlen // .
	const (
		trueVal  = "true"
		falseVal = "false"
	)

	for _, country := range countries(c.Countries) {
		if result, err = c.Scraper.Scrape(ctx, target,
			webScraperOptions{
				Retry: twitterRetryFn,
				Options: func(options map[string]string) map[string]string {
					options["country"] = country
					options["block_resources"] = falseVal
					options["load_iframes"] = trueVal
					options["load_shadowroots"] = falseVal
					options["wait_for_css"] = "#post-detail"

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
