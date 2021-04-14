// Derived from https://raw.githubusercontent.com/prometheus/prometheus/1cffda5de775668f7438578c90a05e8bfa58eb91/web/ui/react-app/src/Navbar.tsx.
// License: https://github.com/prometheus/prometheus/blob/1cffda5de775668f7438578c90a05e8bfa58eb91/LICENSE

import React, { FC, useState } from 'react';
import { Link } from '@reach/router';
import {
  Navbar,
  NavbarToggler,
} from 'reactstrap';
import { usePathPrefix } from './contexts/PathPrefixContext';

interface NavbarProps {
  consolesLink: string | null;
}

const Navigation: FC<NavbarProps> = ({ consolesLink }) => {
  const [isOpen, setIsOpen] = useState(false);
  const toggle = () => setIsOpen(!isOpen);
  const pathPrefix = usePathPrefix();
  return (
    <Navbar className="mb-3" dark color="dark" expand="md" fixed="top">
      <NavbarToggler onClick={toggle} />
      <Link className="pt-0 navbar-brand" to={`${pathPrefix}/graph`}>
        Google Cloud Prometheus Engine
      </Link>
    </Navbar>
  );
};

export default Navigation;

