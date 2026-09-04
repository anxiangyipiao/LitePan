package httpx

import "litepan/internal/domain"

func WrapTransportError(err error) error {
	if err == nil {
		return nil
	}
	return domain.Wrap(domain.CodeDriverError, err)
}

func OAuthProxyDecodeError(err error) error {
	if err == nil {
		return nil
	}
	return domain.Wrap(domain.CodeDriverError, err)
}
