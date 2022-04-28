// Derived from https://raw.githubusercontent.com/prometheus/prometheus/1cffda5de775668f7438578c90a05e8bfa58eb91/web/ui/react-app/src/Navbar.tsx.
// License: https://github.com/prometheus/prometheus/blob/1cffda5de775668f7438578c90a05e8bfa58eb91/LICENSE

import React, { FC, useState } from 'react';
import { Link } from 'react-router-dom';
import { Navbar, NavbarToggler } from 'reactstrap';
import { ThemeToggle } from './Theme';
import logo from './images/prometheus_logo_grey.svg';

interface NavbarProps {
  consolesLink: string | null;
  agentMode: boolean;
}

const Navigation: FC<NavbarProps> = ({ consolesLink, agentMode }) => {
  const [isOpen, setIsOpen] = useState(false);
  const toggle = () => setIsOpen(!isOpen);
  return (
    <Navbar className="mb-3" dark color="dark" expand="md" fixed="top">
      <NavbarToggler onClick={toggle} className="mr-2" />
      <Link className="pt-0 navbar-brand" to={'/graph'}>
        <img src={logo} className="d-inline-block align-top" alt="Prometheus logo" title="Prometheus" />
        Google Cloud Managed Service for Prometheus
      </Link>
      <ThemeToggle />
    </Navbar>
  );
};

export default Navigation;
