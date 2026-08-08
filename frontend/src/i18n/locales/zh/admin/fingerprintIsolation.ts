export default {
  fingerprintIsolation: {
    eyebrow: '连接运行面',
    title: '指纹与连接',
    description: '集中查看 TLS 模板、账号绑定关系和出站连接诊断。模板编辑沿用现有管理弹窗。',
    manageProfiles: '管理 TLS 模板',
    accountsTitle: '账号绑定概览',
    accountsDescription: '按账号查看稳定模板绑定、代理范围和连接诊断入口。',
    searchAccounts: '搜索账号名称',
    noAccounts: '暂无账号',
    noAccountsHint: '请先在账号管理中添加账号，再在这里查看绑定状态。',
    noSearchResults: '没有匹配的账号',
    noSearchResultsHint: '请更换搜索词后重试。',
    unbound: '未绑定 TLS 模板',
    direct: '直连',
    diagnose: '连接诊断',
    openDiagnostic: '打开账号连接诊断',
    loadProfilesFailed: 'TLS 模板加载失败',
    loadAccountsFailed: '账号列表加载失败',
    footerHint: '连接诊断只读取网络路径和进程内 Transport 统计，不会显示账号凭据。',
    columns: {
      account: '账号',
      platform: '平台 / 类型',
      status: '状态',
      profile: 'TLS 模板',
      proxy: '代理',
      actions: '操作'
    },
    stats: {
      profiles: 'TLS 模板',
      profilesDetail: '可用于账号绑定',
      accounts: '账号总数',
      accountsDetail: '来自账号管理分页数据',
      bound: '当前页已绑定',
      boundDetail: '启用且找到对应模板',
      diagnostic: '当前页已启用',
      diagnosticDetail: '可查看稳定连接配置'
    }
  }
}
