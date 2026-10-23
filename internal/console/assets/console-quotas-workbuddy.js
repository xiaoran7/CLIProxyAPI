/**
 * Gate Admin Console - WorkBuddy Package & Expiration Resolution
 * WorkBuddy 资源包与到期时间解析（余额、积分、套餐名归一化）。
 */

    function getWorkBuddyExpirationInfo(acc) {
      if (!acc) {
        return {
          hasExpiration: false,
          shortText: t('quotas.workbuddy.expiryUnannounced'),
          fullText: t('quotas.workbuddy.expiryUnannounced'),
          countdown: '',
          color: 'var(--text-dim)',
          tooltip: t('quotas.workbuddy.expiryUnannouncedTooltip'),
          packages: [],
          hasMultiplePackages: false,
          packageCount: 0
        };
      }

      // 提取所有候选包明细（优先 acc.expiration_packages，其次 acc.groups 各 buckets）
      let rawPackages = [];
      if (Array.isArray(acc.expiration_packages) && acc.expiration_packages.length > 0) {
        rawPackages = acc.expiration_packages;
      } else if (Array.isArray(acc.groups)) {
        for (const g of acc.groups) {
          for (const b of (g.buckets || [])) {
            if (b.expires_at || b.reset_time) {
              rawPackages.push({
                package_name: b.name,
                package_code: b.package_code || b.bucket_id,
                remain_points: b.remain_capacity != null ? Number(b.remain_capacity) : 0,
                total_points: b.total_capacity != null ? Number(b.total_capacity) : 0,
                expires_at: b.expires_at || '',
                reset_time: b.reset_time || ''
              });
            }
          }
        }
      }

      // 如果未收集到包列表，但顶层有 expires_at / reset_time，构建兜底单包
      if (rawPackages.length === 0 && (acc.expires_at || acc.reset_time)) {
        rawPackages.push({
          package_name: t('quotas.workbuddy.defaultPackName'),
          package_code: '',
          remain_points: acc.runs_out_points != null ? Number(acc.runs_out_points) : (acc.remaining_credits != null ? Number(acc.remaining_credits) : 0),
          total_points: acc.total_credits != null ? Number(acc.total_credits) : 0,
          expires_at: acc.expires_at || '',
          reset_time: acc.reset_time || ''
        });
      }

      if (rawPackages.length === 0) {
        return {
          hasExpiration: false,
          shortText: t('quotas.workbuddy.expiryUnannounced'),
          fullText: t('quotas.workbuddy.expiryUnannounced'),
          countdown: '',
          color: 'var(--text-dim)',
          tooltip: t('quotas.workbuddy.expiryUnannouncedTooltip'),
          packages: [],
          hasMultiplePackages: false,
          packageCount: 0
        };
      }

      const now = Date.now();
      const formattedPackages = [];

      for (const p of rawPackages) {
        let targetTs = null;
        if (p.reset_time) {
          try {
            const d = new Date(p.reset_time);
            if (!isNaN(d.getTime())) targetTs = d.getTime();
          } catch (_) {}
        }
        if (!targetTs && p.expires_at) {
          try {
            const s = String(p.expires_at).trim();
            const norm = /(?:Z|[+-]\d{2}(?::?\d{2})?)$/i.test(s) ? s : s.replace(' ', 'T') + '+08:00';
            const d = new Date(norm);
            if (!isNaN(d.getTime())) targetTs = d.getTime();
          } catch (_) {}
        }

        let shortText = '';
        let fullText = p.expires_at || '';
        const currentCstYear = new Date(now + 8 * 3600 * 1000).getUTCFullYear();
        if (targetTs) {
          // 全局东八区单点真理源：统一转为东八区（UTC+8 北京时间）并使用 UTC getter 提取，避免多时区偏移
          const cstDate = new Date(targetTs + 8 * 3600 * 1000);
          const yyyy = cstDate.getUTCFullYear();
          const mm = cstDate.getUTCMonth() + 1;
          const dd = cstDate.getUTCDate();
          const hh = String(cstDate.getUTCHours()).padStart(2, '0');
          const min = String(cstDate.getUTCMinutes()).padStart(2, '0');
          if (yyyy !== currentCstYear) {
            shortText = t('quotas.points.expiryDateTime', { yyyy: yyyy, mm: mm, dd: dd, hh: hh, min: min });
          } else {
            shortText = t('quotas.points.expiryShortDate', { mm: mm, dd: dd, hh: hh, min: min });
          }
          if (!fullText) {
            const ss = String(cstDate.getUTCSeconds()).padStart(2, '0');
            fullText = `${yyyy}-${String(mm).padStart(2, '0')}-${String(dd).padStart(2, '0')} ${hh}:${min}:${ss}`;
          }
        } else if (p.expires_at) {
          const s = String(p.expires_at).trim();
          const m = s.match(/^(\d{4})-(\d{1,2})-(\d{1,2})/);
          if (m && Number(m[1]) !== currentCstYear) {
            const tm = s.slice(11, 16);
            shortText = t('quotas.points.expiryFullDate', { value: m[1], value2: Number(m[2]), value3: Number(m[3]), value4: tm ? ' ' + tm : '' }).trim();
          } else {
            shortText = s.slice(5, 16).replace('T', ' ');
          }
        } else {
          shortText = t('quotas.workbuddy.unknownExpiry');
          fullText = t('quotas.workbuddy.unknownExpiry');
        }

        let countdown = '';
        let color = 'var(--text-muted)';
        let isLongTerm = false;
        const pkgRawName = p.package_name || p.name || '';
        // i18n-exempt: 匹配上游 API 返回的资源包原始名（含中文），与 lib/upstream/sync.mjs 同口径，不得翻译
        const isFreeOrTrialName = /free plan|体验|免费/i.test(pkgRawName);

        if (targetTs) {
          const diff = targetTs - now;
          if (diff <= 0) {
            countdown = t('quotas.points.expired');
            color = '#ef4444';
          } else {
            const hours = Math.floor(diff / (1000 * 3600));
            const days = Math.floor(hours / 24);
            // 识别远期基础包（如腾讯 2035 年代表永久有效的 Free Plan Subscription 或体验版）
            if (days >= 365 * 3 || (isFreeOrTrialName && days >= 365)) {
              isLongTerm = true;
              shortText = t('quotas.workbuddy.longTerm');
              countdown = '';
              color = '#10b981';
            } else if (hours < 24) {
              countdown = t('quotas.points.expiresInHours', { value: Math.max(1, hours) });
              color = '#ef4444';
            } else if (hours < 72) {
              countdown = t('quotas.points.expiresInDays', { days: days });
              color = '#f59e0b';
            } else {
              if (days < 365) {
                countdown = t('quotas.points.expiresInDays', { days: days });
              } else {
                const years = Math.floor(days / 365);
                const remMonths = Math.floor((days % 365) / 30);
                countdown = remMonths >= 1 ? t('quotas.points.expiresInYearsMonths', { years: years, remMonths: remMonths }) : t('quotas.points.expiresInYears', { years: years });
              }
              color = 'var(--text-muted)';
            }
          }
        } else if (p.expires_at) {
          const m = String(p.expires_at).trim().match(/^(\d{4})/);
          if (m && (Number(m[1]) >= currentCstYear + 3 || (isFreeOrTrialName && Number(m[1]) > currentCstYear))) {
            isLongTerm = true;
            shortText = t('quotas.workbuddy.longTerm');
            countdown = '';
            color = '#10b981';
          }
        }

        formattedPackages.push({
          name: p.package_name || p.name || t('quotas.workbuddy.defaultPackName'),
          code: p.package_code || '',
          points: Number(p.remain_points ?? p.remain_capacity ?? 0),
          totalPoints: Number(p.total_points ?? p.total_capacity ?? 0),
          targetTs,
          shortText,
          fullText,
          countdown,
          color,
          isLongTerm,
          hasExpiryText: Boolean(targetTs || p.expires_at)
        });
      }

      // 按到期时间排序（最早到期的排前面，有到期但无时间戳的其次，完全无到期时间的排末尾）
      formattedPackages.sort((a, b) => {
        if (a.targetTs && b.targetTs) return a.targetTs - b.targetTs;
        if (a.targetTs) return -1;
        if (b.targetTs) return 1;
        if (a.hasExpiryText && !b.hasExpiryText) return -1;
        if (!a.hasExpiryText && b.hasExpiryText) return 1;
        return 0;
      });

      // 检查是否有任何包包含有效到期时间（无论是 targetTs 还是非空的 expires_at）
      const hasAnyExpiration = formattedPackages.some(p => p.targetTs != null || p.hasExpiryText);
      if (!hasAnyExpiration) {
        return {
          hasExpiration: false,
          shortText: t('quotas.workbuddy.expiryUnannounced'),
          fullText: t('quotas.workbuddy.expiryUnannounced'),
          countdown: '',
          color: 'var(--text-dim)',
          tooltip: t('quotas.workbuddy.expiryUnannouncedTooltip'),
          packages: formattedPackages,
          hasMultiplePackages: false,
          packageCount: formattedPackages.length
        };
      }

      const earliest = formattedPackages[0];
      const hasMultiplePackages = formattedPackages.length > 1;
      const isAllLongTerm = hasAnyExpiration && formattedPackages.every(p => p.isLongTerm);

      let tooltip = '';
      if (!hasMultiplePackages) {
        tooltip = t('quotas.workbuddy.pointsExpiryTooltip', { fullText: earliest.fullText });
        if (earliest.points > 0) {
          tooltip += ` ${t('quotas.points.availablePointsSuffix', { points: earliest.points })}`;
        }
        if (earliest.isLongTerm) {
          tooltip += ` ${t('quotas.workbuddy.longTermSuffix')}`;
        }
      } else {
        tooltip = `${t('quotas.workbuddy.pointsBatchExpiryTooltip', { count: formattedPackages.length })}\n` +
          formattedPackages.map((p, idx) =>
            `${t('quotas.workbuddy.batchLine', { value: idx + 1, name: p.name, points: p.points, fullText: p.fullText })}${p.countdown ? ` (${p.countdown})` : (p.isLongTerm ? ` ${t('quotas.workbuddy.longTermParenthetical')}` : '')}`
          ).join('\n');
      }

      return {
        hasExpiration: true,
        targetTs: earliest.targetTs,
        shortText: isAllLongTerm ? t('quotas.workbuddy.longTerm') : earliest.shortText,
        fullText: earliest.fullText,
        countdown: isAllLongTerm ? '' : earliest.countdown,
        color: isAllLongTerm ? '#10b981' : earliest.color,
        tooltip,
        runsOutPoints: earliest.points,
        packages: formattedPackages,
        hasMultiplePackages,
        packageCount: formattedPackages.length,
        isAllLongTerm,
        isLongTerm: earliest.isLongTerm
      };
    }
