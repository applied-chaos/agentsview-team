alter table hosts drop constraint hosts_install_mode_check;
alter table hosts add constraint hosts_install_mode_check
    check (install_mode in ('systemd', 'profile', 'launchd', 'task'));
