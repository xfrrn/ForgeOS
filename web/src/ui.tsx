import type { ComponentProps, ReactNode } from 'react'
import { Icon } from './shared'

type IconName = ComponentProps<typeof Icon>['name']

export function BackLink({ href, children }: { href: string; children: ReactNode }) {
  return <a className="back-link" href={href}><Icon name="ArrowLeft" size={14}/>{children}</a>
}

export function PageHeading({ title, description, actions }: {
  title: string; description?: ReactNode; actions?: ReactNode
}) {
  return <div className="page-heading">
    <div>
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
    {actions && <div className="page-actions">{actions}</div>}
  </div>
}

type MetricDestination = { href: string; onClick?: never; pressed?: never } |
  { href?: never; onClick: () => void; pressed?: boolean }

export function MetricCard({ label, value, icon, href, onClick, pressed }: {
  label: string; value: number | string; icon: IconName
} & MetricDestination) {
  const content = <>
    <span className="metric-label">{label}</span>
    <span className="metric-icon"><Icon name={icon} size={18}/></span>
    <strong className="metric-value">{value}</strong>
  </>
  return href !== undefined ? <a className="panel metric" href={href}>{content}</a> :
    <button type="button" className="panel metric" onClick={onClick} aria-pressed={pressed}>{content}</button>
}

export function EmptyState({ icon, title, description, children }: {
  icon: IconName; title: string; description?: string; children?: ReactNode
}) {
  return <div className="empty-state">
    <span className="empty-icon"><Icon name={icon} size={22}/></span>
    <strong>{title}</strong>
    {description && <p>{description}</p>}
    {children}
  </div>
}

export function SkeletonLines({ lines = 3 }: { lines?: number }) {
  return <div className="skeleton-stack" aria-hidden="true">
    {Array.from({ length: lines }, (_, index) => (
      <span key={index} className={'skeleton-line' + (index === 0 ? ' wide' : index === lines - 1 ? ' short' : '')}/>
    ))}
  </div>
}

export function SkeletonCard({ label = '正在读取…', lines = 3 }: { label?: string; lines?: number }) {
  return <div className="panel skeleton-card" role="status" aria-label={label}>
    <span className="sr-only">{label}</span>
    <SkeletonLines lines={lines}/>
  </div>
}
