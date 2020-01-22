#ifndef QMLENVIRONMENTVARIABLE_H
#define QMLENVIRONMENTVARIABLE_H

#include <QObject>

class QQmlEngine;
class QJSEngine;

class QmlEnvironmentVariable : public QObject
{
    Q_OBJECT
public:
    Q_INVOKABLE static QString value(const QString &name);
    Q_INVOKABLE static QString value(const QString &name, const QString &fallback);
    Q_INVOKABLE static void setValue(const QString &name, const QString &value);
    Q_INVOKABLE static void unset(const QString &name);
};

// Define the singleton type provider function (callback).
QObject *qmlenvironmentvariable_singletontype_provider(QQmlEngine *, QJSEngine *);

#endif // QMLENVIRONMENTVARIABLE_H