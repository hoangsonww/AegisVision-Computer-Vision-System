'use client';

import { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { LoadingState, EmptyState, ErrorState } from './Primitives';

export interface Column<T> {
  header: string;
  accessor: (row: T, idx: number) => ReactNode;
  className?: string;
  width?: string;
}

interface DataTableProps<T> {
  columns: Column<T>[];
  rows: T[] | undefined;
  isLoading?: boolean;
  error?: unknown;
  emptyTitle?: string;
  emptyHint?: string;
  onRowClick?: (row: T) => void;
  rowKey?: (row: T, idx: number) => string;
  className?: string;
}

export function DataTable<T>({
  columns,
  rows,
  isLoading,
  error,
  emptyTitle = 'No items',
  emptyHint,
  onRowClick,
  rowKey,
  className,
}: DataTableProps<T>) {
  if (isLoading && !rows) return <LoadingState />;
  if (error) return <ErrorState error={error} />;
  if (!rows || rows.length === 0) return <EmptyState title={emptyTitle} hint={emptyHint} />;

  return (
    <div className={cn('card overflow-hidden p-0', className)}>
      <div className="overflow-x-auto">
        <table className="table-base">
          <thead>
            <tr>
              {columns.map((c, i) => (
                <th key={i} className={c.className} style={c.width ? { width: c.width } : undefined}>
                  {c.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, idx) => (
              <tr
                key={rowKey ? rowKey(row, idx) : idx}
                className={onRowClick ? 'cursor-pointer' : undefined}
                onClick={() => onRowClick?.(row)}
              >
                {columns.map((c, i) => (
                  <td key={i} className={c.className}>
                    {c.accessor(row, idx)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
