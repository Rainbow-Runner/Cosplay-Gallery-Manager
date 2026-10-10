import { ApolloProvider } from "@apollo/client/react";
import type { PropsWithChildren } from "react";
import { IntlProvider } from "react-intl";

import { apolloClient } from "../api/apollo";
import { messages, resolveLocale } from "../i18n/messages";

export function Providers({ children }: PropsWithChildren) {
  const locale = resolveLocale(navigator.language);

  return (
    <ApolloProvider client={apolloClient}>
      <IntlProvider locale={locale} messages={messages[locale]}>
        {children}
      </IntlProvider>
    </ApolloProvider>
  );
}
