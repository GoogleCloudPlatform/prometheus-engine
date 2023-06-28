// Derived from https://raw.githubusercontent.com/prometheus/prometheus/v2.45.0/web/ui/react-app/src/Navbar.tsx
// NOTE(bwplotka): This override changes title to "Google Cloud Managed Service
// for Prometheus", removes agent option handling and collapsible links.
// License: https://github.com/prometheus/prometheus/blob/v2.45.0/LICENSE

import React, { FC, useState } from 'react';
import { Link } from 'react-router-dom';
import { Navbar, NavbarToggler } from 'reactstrap';
import { ThemeToggle } from './Theme';
import { ReactComponent as PromLogo } from './images/prometheus_logo_grey.svg';

interface NavbarProps {
  consolesLink: string | null;
  agentMode: boolean;
  animateLogo?: boolean | false;
}

const Navigation: FC<NavbarProps> = ({ consolesLink, agentMode, animateLogo }) => {
  const [isOpen, setIsOpen] = useState(false);
  const toggle = () => setIsOpen(!isOpen);
  return (
    <Navbar className="mb-3" dark color="dark" expand="md" fixed="top">
      <NavbarToggler onClick={toggle} className="mr-2" />
      <Link className="pt-0 navbar-brand" to={'/graph'}>
        <PromLogo className={`d-inline-block align-top${animateLogo ? ' animate' : ''}`} title="Prometheus" />
        Google Cloud Managed Service for Prometheus
      </Link>
      <ThemeToggle />
    </Navbar>
  );
};

export default Navigation;
