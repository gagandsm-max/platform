package com.platform.core.sensorcontrol.pollgrant;

import com.platform.core.sensorcontrol.SensorControlException;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.datasource.ConnectionHolder;
import org.springframework.transaction.support.TransactionSynchronizationManager;

import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.SQLException;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class SensorPollGrantTransactionGuardTest {
    private final DataSource dataSource = mock(DataSource.class);
    private final DataSource otherDataSource = mock(DataSource.class);
    private final Connection connection = mock(Connection.class);
    private final SensorPollGrantRepository repo = new SensorPollGrantRepository(
            new NamedParameterJdbcTemplate(dataSource));

    @AfterEach
    void clearOwnedTransactionState() {
        try {
            TransactionSynchronizationManager.unbindResourceIfPossible(dataSource);
            TransactionSynchronizationManager.unbindResourceIfPossible(otherDataSource);
        } finally {
            TransactionSynchronizationManager.clear();
        }
    }

    @Test
    void rejectsNoTransactionBeforeObtainingAConnectionOrQuerying() {
        assertRejected("unlocked lookup");
        verifyNoInteractions(connection);
    }

    @Test
    void threadTransactionFlagAloneDoesNotEstablishParticipation() {
        TransactionSynchronizationManager.setActualTransactionActive(true);
        assertRejected("not bound");
        verifyNoInteractions(connection);
    }

    @Test
    void synchronizedBindingWithoutActualTransactionIsInsufficient() {
        bind(dataSource, true);
        assertRejected("unlocked lookup");
        verifyNoInteractions(connection);
    }

    @Test
    void wrongDataSourceBindingDoesNotAuthorizeRepositoryQuery() {
        TransactionSynchronizationManager.setActualTransactionActive(true);
        bind(otherDataSource, true);
        assertRejected("not bound");
        verifyNoInteractions(connection);
    }

    @Test
    void boundButUnsynchronizedHolderIsRejectedBeforeQuery() {
        TransactionSynchronizationManager.setActualTransactionActive(true);
        bind(dataSource, false);
        assertRejected("not synchronized");
        verifyNoInteractions(connection);
    }

    @Test
    void synchronizedHolderWithoutConnectionIsRejectedBeforeQuery() {
        TransactionSynchronizationManager.setActualTransactionActive(true);
        ConnectionHolder holder = new ConnectionHolder((Connection) null);
        holder.setSynchronizedWithTransaction(true);
        TransactionSynchronizationManager.bindResource(dataSource, holder);
        assertRejected("not bound");
        verifyNoInteractions(connection);
    }

    @Test
    void boundSynchronizedAutocommitConnectionIsRejectedWithoutQueryOrRelease() throws Exception {
        TransactionSynchronizationManager.setActualTransactionActive(true);
        bind(dataSource, true);
        when(connection.getAutoCommit()).thenReturn(true);
        assertRejected("autocommit");
        verify(connection).getAutoCommit();
        verifyNoMoreInteractions(connection);
    }

    @Test
    void inabilityToReadAutocommitFailsClosedWithoutQueryOrRelease() throws Exception {
        TransactionSynchronizationManager.setActualTransactionActive(true);
        bind(dataSource, true);
        when(connection.getAutoCommit()).thenThrow(new SQLException("fixture failure"));
        assertRejected("unable to verify");
        verify(connection).getAutoCommit();
        verifyNoMoreInteractions(connection);
    }

    private void bind(DataSource source, boolean synchronizedWithTransaction) {
        ConnectionHolder holder = new ConnectionHolder(connection);
        holder.setSynchronizedWithTransaction(synchronizedWithTransaction);
        TransactionSynchronizationManager.bindResource(source, holder);
    }

    private void assertRejected(String reason) {
        SensorControlException failure = assertThrows(SensorControlException.class,
                () -> repo.findForShare(UUID.randomUUID(), "fixture-sensor", UUID.randomUUID()));
        assertEquals(SensorControlException.ERR_SENSOR_STATE_INVALID, failure.getCode());
        assertTrue(failure.getMessage().contains("findForShare"));
        assertTrue(failure.getMessage().contains(reason));
        // No fallback connection, replacement transaction, SQL or caller-connection release.
        verifyNoInteractions(dataSource, otherDataSource);
    }
}
