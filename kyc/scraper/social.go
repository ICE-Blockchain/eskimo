// SPDX-License-Identifier: ice License 1.0

package scraper

import (
	"fmt"
	"os"
	"strings"

	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/log"
)

func loadConfig() *config {
	var cfg config

	appcfg.MustLoadFromKey(applicationYAMLKey, &cfg)

	for ptr, env := range map[*string]string{
		&cfg.WebScrapingAPI.APIKeyV1:        os.Getenv("WEB_SCRAPING_API_KEY_V1"),
		&cfg.WebScrapingAPI.APIKeyV2:        os.Getenv("WEB_SCRAPING_API_KEY_V2"),
		&cfg.WebScrapingAPI.BaseURL:         os.Getenv("WEB_SCRAPING_API_BASE_URL"),
		&cfg.SocialLinks.Facebook.AppID:     os.Getenv("FACEBOOK_APP_ID"),
		&cfg.SocialLinks.Facebook.AppSecret: os.Getenv("FACEBOOK_APP_SECRET"),
	} {
		if *ptr == "" {
			*ptr = env
		}
	}

	return &cfg
}

func New(st StrategyType) Verifier {
	conf := loadConfig()
	conf.WebScrapingAPI.BaseURL = strings.TrimSuffix(conf.WebScrapingAPI.BaseURL, "/")

	switch st {
	case StrategyTwitter:
		sc := newMustWebScraper(fmt.Sprintf("%v/%v", conf.WebScrapingAPI.BaseURL, scraperV2Suffix), conf.WebScrapingAPI.APIKeyV2)

		return newTwitterVerifier(sc, conf.SocialLinks.Twitter.Domains, conf.WebScrapingAPI.Countries)

	case StrategyFacebook:
		sc := new(dataFetcherImpl)

		return newFacebookVerifier(
			sc,
			conf.SocialLinks.Facebook.AppID,
			conf.SocialLinks.Facebook.AppSecret,
			conf.SocialLinks.Facebook.AllowLongLiveTokens,
		)

	case StrategyCMC:
		sc := newMustWebScraper(fmt.Sprintf("%v/%v", conf.WebScrapingAPI.BaseURL, scraperV1Suffix), conf.WebScrapingAPI.APIKeyV1)

		return newCMCVerifier(sc, conf.WebScrapingAPI.Countries)
	default:
		log.Panic("invalid social verifier: " + st)
	}

	return nil
}
