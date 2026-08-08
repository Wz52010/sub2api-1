export default {
  fingerprintIsolation: {
    eyebrow: 'Connection runtime',
    title: 'Fingerprint & Connections',
    description: 'Review TLS profiles, account bindings, and outbound connection diagnostics in one place. Profile editing reuses the existing admin dialog.',
    manageProfiles: 'Manage TLS profiles',
    accountsTitle: 'Account bindings',
    accountsDescription: 'Inspect stable per-account profile bindings, proxy scope, and connection diagnostics.',
    searchAccounts: 'Search account name',
    noAccounts: 'No accounts yet',
    noAccountsHint: 'Add an account in Account Management to review its binding here.',
    noSearchResults: 'No matching accounts',
    noSearchResultsHint: 'Try a different search term.',
    unbound: 'No TLS profile bound',
    direct: 'Direct',
    diagnose: 'Diagnose',
    openDiagnostic: 'Open account connection diagnostic',
    loadProfilesFailed: 'Failed to load TLS profiles',
    loadAccountsFailed: 'Failed to load accounts',
    footerHint: 'Diagnostics read network-path and process-local Transport data only; account credentials are not displayed.',
    columns: {
      account: 'Account',
      platform: 'Platform / type',
      status: 'Status',
      profile: 'TLS profile',
      proxy: 'Proxy',
      actions: 'Actions'
    },
    stats: {
      profiles: 'TLS profiles',
      profilesDetail: 'Available for account binding',
      accounts: 'Total accounts',
      accountsDetail: 'From paginated account data',
      bound: 'Bound on page',
      boundDetail: 'Enabled with a matching profile',
      diagnostic: 'Enabled on page',
      diagnosticDetail: 'Connection configuration available'
    }
  }
}
