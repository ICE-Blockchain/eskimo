// SPDX-License-Identifier: ice License 1.0

package scraper

import (
	"os"

	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/log"
)

func loadConfig() *config {
	var cfg config

	appcfg.MustLoadFromKey(applicationYAMLKey, &cfg)

	for ptr, env := range map[*string]string{
		&cfg.WebScrapingAPI.APIKeyV1:        os.Getenv("WEB_SCRAPING_API_KEY_V1"),
		&cfg.WebScrapingAPI.URLV1:           os.Getenv("WEB_SCRAPING_API_URL_V1"),
		&cfg.WebScrapingAPI.APIKeyV2:        os.Getenv("WEB_SCRAPING_API_KEY_V2"),
		&cfg.WebScrapingAPI.URLV2:           os.Getenv("WEB_SCRAPING_API_URL_V2"),
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

	switch st {
	case StrategyTwitter:
		sc := newMustWebScraper(conf.WebScrapingAPI.URLV2, conf.WebScrapingAPI.APIKeyV2)

		return newTwitterVerifier(sc, conf.SocialLinks.Twitter.Domains, conf.SocialLinks.Twitter.Countries)

	case StrategyFacebook:
		sc := new(dataFetcherImpl)

		return newFacebookVerifier(
			sc,
			conf.SocialLinks.Facebook.AppID,
			conf.SocialLinks.Facebook.AppSecret,
			conf.SocialLinks.Facebook.AllowLongLiveTokens,
		)

	case StrategyCMC:
		sc := newMustWebScraper(conf.WebScrapingAPI.URLV1, conf.WebScrapingAPI.APIKeyV1)

		return newCMCVerifier(sc, conf.SocialLinks.CMC.Countries)
	default:
		log.Panic("invalid social verifier: " + st)
	}

	return nil
}
