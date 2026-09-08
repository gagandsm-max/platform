import { Component, ReactNode } from 'react';
import { PageState } from './PageState';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
  };

  public static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  public render() {
    if (this.state.hasError) {
      return (
        <main className="main-content" id="main-content">
          <PageState
            kind="error"
            title="Platform UI encountered an unexpected error"
            message="The application shell could not render this view. No raw exception details are exposed in the UI."
            actionLabel="Reload page"
            onAction={() => window.location.reload()}
          />
        </main>
      );
    }

    return this.props.children;
  }
}
