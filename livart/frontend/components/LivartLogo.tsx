import React, { useEffect, useState } from 'react';

type PublicSettingsResponse = {
  data?: {
    site_logo?: string;
  };
};

const sub2apiBaseUrl = /^(localhost|127\.0\.0\.1)$/.test(window.location.hostname)
  ? 'http://localhost:18080'
  : 'https://api.cc2.cx';
let siteLogoPromise: Promise<string> | undefined;

function loadSiteLogo(): Promise<string> {
  if (!siteLogoPromise) {
    siteLogoPromise = fetch(`${sub2apiBaseUrl}/api/v1/settings/public`, { credentials: 'omit', cache: 'no-store' })
      .then(async (response) => {
        if (!response.ok) return '';
        const settings = await response.json() as PublicSettingsResponse;
        const logo = settings.data?.site_logo?.trim();
        return logo ? new URL(logo, `${sub2apiBaseUrl}/`).href : `${sub2apiBaseUrl}/logo.jpg`;
      })
      .catch(() => `${sub2apiBaseUrl}/logo.jpg`);
  }
  return siteLogoPromise;
}

interface LivartLogoProps {
  size?: number;
  className?: string;
  title?: string;
}

const LivartLogo: React.FC<LivartLogoProps> = ({
  size = 40,
  className = '',
  title = '超级改图'
}) => {
  const [src, setSrc] = useState('/project-icon.jpg');

  useEffect(() => {
    let mounted = true;
    const syncSiteLogo = () => {
      const configuredLogo = (window as Window & { __SUB2API_SITE_LOGO__?: string }).__SUB2API_SITE_LOGO__;
      if (configuredLogo) {
        setSrc(configuredLogo);
        return;
      }
      void loadSiteLogo().then((logo) => {
        if (mounted && logo) setSrc(logo);
      });
    };
    syncSiteLogo();
    window.addEventListener('sub2api-logo-updated', syncSiteLogo);
    return () => {
      mounted = false;
      window.removeEventListener('sub2api-logo-updated', syncSiteLogo);
    };
  }, []);

  return (
  <img
    src={src}
    onError={() => setSrc('/project-icon.jpg')}
    width={size}
    height={size}
    role="img"
    aria-label={title}
    className={`rounded-[18px] object-cover ${className}`}
    alt={title}
  />
  );
};

export default LivartLogo;
