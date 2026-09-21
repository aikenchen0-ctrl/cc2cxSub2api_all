import React from 'react';

interface LivartLogoProps {
  size?: number;
  className?: string;
  title?: string;
}

const LivartLogo: React.FC<LivartLogoProps> = ({
  size = 40,
  className = '',
  title = '超级改图'
}) => (
  <img
    src="/project-icon.jpg"
    width={size}
    height={size}
    role="img"
    aria-label={title}
    className={`rounded-[18px] object-cover ${className}`}
    alt={title}
  />
);

export default LivartLogo;
