import { Icon } from "./Icon";

interface AppLogoProps {
  subtitle?: string;
}

export function AppLogo({ subtitle }: AppLogoProps) {
  return (
    <span className="cgm-app-logo">
      <span className="cgm-app-logo__mark" aria-hidden="true"><Icon name="gallery" /></span>
      <span className="cgm-app-logo__text">
        <span className="cgm-app-logo__name">Cosplay Gallery Manager</span>
        {subtitle ? <span className="cgm-app-logo__subtitle">{subtitle}</span> : null}
      </span>
    </span>
  );
}
