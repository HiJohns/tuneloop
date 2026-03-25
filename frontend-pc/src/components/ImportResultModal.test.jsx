import { render, screen } from '@testing-library/react'
import ImportResultModal from './ImportResultModal'

describe('ImportResultModal', () => {
  const defaultProps = {
    visible: true,
    onClose: jest.fn(),
    importResult: {
      success: 95,
      failed: 5,
      total: 100,
      errors: ['Row 10: Missing required field: name']
    }
  }

  test('renders statistics dashboard correctly', () => {
    render(<ImportResultModal {...defaultProps} />)

    expect(screen.getByText('总计')).toBeInTheDocument()
    expect(screen.getByText('成功')).toBeInTheDocument()
    expect(screen.getByText('失败')).toBeInTheDocument()

    expect(screen.getByText('100')).toBeInTheDocument()
    expect(screen.getByText('95')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
  })

  test('calculates and displays success rate', () => {
    render(<ImportResultModal {...defaultProps} />)

    expect(screen.getByText('成功率: 95.0%')).toBeInTheDocument()
  })

  test('displays error details when errors exist', () => {
    render(<ImportResultModal {...defaultProps} />)

    expect(screen.getByText('错误详情:')).toBeInTheDocument()
    expect(screen.getByText('Row 10: Missing required field: name')).toBeInTheDocument()
  })

  test('shows success alert when no errors', () => {
    const successProps = {
      ...defaultProps,
      importResult: {
        success: 100,
        failed: 0,
        total: 100,
        errors: []
      }
    }

    render(<ImportResultModal {...successProps} />)

    expect(screen.getByText('导入成功')).toBeInTheDocument()
    expect(screen.getByText('所有数据已成功导入系统')).toBeInTheDocument()
  })

  test('handles empty importResult gracefully', () => {
    const { container } = render(<ImportResultModal visible={true} onClose={jest.fn()} />)
    
    expect(container.firstChild).toBeNull()
  })

  test('does not render when not visible', () => {
    const { container } = render(
      <ImportResultModal visible={false} onClose={jest.fn()} importResult={defaultProps.importResult} />
    )
    
    // Modal should not be rendered when not visible
    expect(container.firstChild).toBeNull()
  })

  test('renders error list with correct styling', () => {
    const errors = [
      'Row 5: Invalid price format',
      'Row 12: Missing required field: brand',
      'Row 20: Invalid status value'
    ]

    render(
      <ImportResultModal
        visible={true}
        onClose={jest.fn()}
        importResult={{ success: 97, failed: 3, total: 100, errors }}
      />
    )

    errors.forEach(error => {
      expect(screen.getByText(error)).toBeInTheDocument()
    })
  })

  test('displays correct success rate calculation for edge cases', () => {
    const edgeCases = [
      { total: 0, success: 0, failed: 0, expectedRate: '0' },
      { total: 1, success: 1, failed: 0, expectedRate: '100.0' },
      { total: 3, success: 1, failed: 2, expectedRate: '33.3' }
    ]

    edgeCases.forEach(({ total, success, failed, expectedRate }) => {
      const { rerender } = render(
        <ImportResultModal
          visible={true}
          onClose={jest.fn()}
          importResult={{ success, failed, total, errors: [] }}
        />
      )

      expect(screen.getByText(`成功率: ${expectedRate}%`)).toBeInTheDocument()
    })
  })
})
