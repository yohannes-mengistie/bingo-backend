#!/bin/bash

# Script to view Render PostgreSQL database data
# Usage: ./view_render_db.sh [command]

# Render PostgreSQL connection string
RENDER_DB_URL="postgresql://biruh_bingo_user:wE7cJMXCkXqeug3bKH6m9w58OHfr2PDs@dpg-d6088c7pm1nc73d571j0-a.virginia-postgres.render.com/biruh_bingo"

# Check if psql is available
if ! command -v psql &> /dev/null; then
    echo "Error: psql is not installed or not in PATH"
    echo "Install PostgreSQL client tools to use this script"
    exit 1
fi

# Test connection
test_connection() {
    if ! psql "$RENDER_DB_URL" -c "SELECT 1;" > /dev/null 2>&1; then
        echo "Error: Could not connect to Render database"
        exit 1
    fi
}

# Function to display header
show_header() {
    echo "=========================================="
    echo "$1"
    echo "=========================================="
    echo ""
}

case "$1" in
    "stats"|"summary")
        test_connection
        show_header "DATABASE SUMMARY"
        psql "$RENDER_DB_URL" -c "
        SELECT 
            'USERS' as table_name, COUNT(*)::text as record_count FROM users
        UNION ALL
        SELECT 'WALLETS', COUNT(*)::text FROM wallets
        UNION ALL
        SELECT 'TRANSACTIONS', COUNT(*)::text FROM transactions
        UNION ALL
        SELECT 'GAMES', COUNT(*)::text FROM games
        UNION ALL
        SELECT 'GAME_PLAYERS', COUNT(*)::text FROM game_players
        UNION ALL
        SELECT 'DRAWN_NUMBERS', COUNT(*)::text FROM drawn_numbers
        ORDER BY table_name;
        "
        echo ""
        psql "$RENDER_DB_URL" -c "
        SELECT 
            (SELECT COUNT(*) FROM users) as total_users,
            (SELECT COUNT(*) FROM wallets) as total_wallets,
            (SELECT SUM(balance) FROM wallets) as total_balance,
            (SELECT COUNT(*) FROM transactions) as total_transactions,
            (SELECT COUNT(*) FROM games) as total_games;
        "
        ;;
    
    "users")
        test_connection
        show_header "USERS"
        psql "$RENDER_DB_URL" -c "
        SELECT 
            id,
            telegram_id,
            first_name,
            last_name,
            phone_number,
            referal_code,
            role,
            created_at
        FROM users 
        ORDER BY created_at DESC;
        "
        ;;
    
    "wallets")
        test_connection
        show_header "WALLETS"
        psql "$RENDER_DB_URL" -c "
        SELECT 
            w.user_id,
            u.telegram_id,
            u.first_name,
            w.balance,
            w.demo_balance,
            w.updated_at
        FROM wallets w
        JOIN users u ON w.user_id = u.id
        ORDER BY w.updated_at DESC;
        "
        ;;
    
    "transactions")
        test_connection
        show_header "TRANSACTIONS"
        if [ "$2" = "all" ]; then
            psql "$RENDER_DB_URL" -c "
            SELECT 
                id,
                user_id,
                type,
                amount,
                status,
                transaction_type,
                created_at
            FROM transactions 
            ORDER BY created_at DESC;
            "
        else
            psql "$RENDER_DB_URL" -c "
            SELECT 
                id,
                user_id,
                type,
                amount,
                status,
                transaction_type,
                created_at
            FROM transactions 
            ORDER BY created_at DESC 
            LIMIT 20;
            "
        fi
        echo ""
        psql "$RENDER_DB_URL" -c "
        SELECT 
            type,
            status,
            COUNT(*) as count,
            SUM(amount) as total_amount
        FROM transactions 
        GROUP BY type, status 
        ORDER BY type, status;
        "
        ;;
    
    "games")
        test_connection
        show_header "GAMES"
        psql "$RENDER_DB_URL" -c "
        SELECT 
            id,
            game_type,
            state,
            bet_amount,
            min_players,
            player_count,
            prize_pool,
            house_cut,
            winner_id,
            created_at
        FROM games 
        ORDER BY created_at DESC;
        "
        echo ""
        psql "$RENDER_DB_URL" -c "
        SELECT 
            game_type,
            state,
            COUNT(*) as count
        FROM games 
        GROUP BY game_type, state 
        ORDER BY game_type, state;
        "
        ;;
    
    "game-players"|"players")
        test_connection
        show_header "GAME PLAYERS"
        psql "$RENDER_DB_URL" -c "
        SELECT 
            gp.id,
            gp.game_id,
            gp.user_id,
            u.telegram_id,
            u.first_name,
            gp.card_id,
            gp.is_eliminated,
            gp.joined_at
        FROM game_players gp
        JOIN users u ON gp.user_id = u.id
        ORDER BY gp.joined_at DESC;
        "
        ;;
    
    "drawn-numbers"|"numbers")
        test_connection
        show_header "DRAWN NUMBERS"
        psql "$RENDER_DB_URL" -c "
        SELECT 
            id,
            game_id,
            letter,
            number,
            drawn_at
        FROM drawn_numbers 
        ORDER BY drawn_at DESC 
        LIMIT 50;
        "
        ;;
    
    "tables")
        test_connection
        show_header "DATABASE TABLES"
        psql "$RENDER_DB_URL" -c "\dt"
        ;;
    
    "all")
        test_connection
        show_header "COMPLETE DATABASE VIEW"
        echo ""
        
        echo "=== SUMMARY ==="
        ./view_render_db.sh stats
        echo ""
        
        echo "=== USERS ==="
        ./view_render_db.sh users
        echo ""
        
        echo "=== WALLETS ==="
        ./view_render_db.sh wallets
        echo ""
        
        echo "=== TRANSACTIONS ==="
        ./view_render_db.sh transactions
        echo ""
        
        echo "=== GAMES ==="
        ./view_render_db.sh games
        echo ""
        
        echo "=== GAME PLAYERS ==="
        ./view_render_db.sh game-players
        echo ""
        
        echo "=== DRAWN NUMBERS (Last 50) ==="
        ./view_render_db.sh drawn-numbers
        ;;
    
    "query")
        if [ -z "$2" ]; then
            echo "Error: Please provide a SQL query"
            echo "Usage: ./view_render_db.sh query \"SELECT * FROM users;\""
            exit 1
        fi
        test_connection
        psql "$RENDER_DB_URL" -c "$2"
        ;;
    
    "shell"|"interactive")
        test_connection
        echo "Connecting to Render PostgreSQL database..."
        echo "Type '\\q' to exit"
        echo ""
        psql "$RENDER_DB_URL"
        ;;
    
    *)
        echo "Render Database Viewer"
        echo "======================"
        echo ""
        echo "Usage: ./view_render_db.sh [command]"
        echo ""
        echo "Commands:"
        echo "  stats          - Show database summary and statistics"
        echo "  users          - View all users"
        echo "  wallets        - View all wallets with user info"
        echo "  transactions   - View recent transactions (use 'all' for all)"
        echo "  games          - View all games"
        echo "  game-players   - View all game players"
        echo "  drawn-numbers  - View drawn numbers (last 50)"
        echo "  tables         - List all database tables"
        echo "  all            - Show complete database view"
        echo "  query \"SQL\"    - Run a custom SQL query"
        echo "  shell          - Open interactive psql shell"
        echo ""
        echo "Examples:"
        echo "  ./view_render_db.sh stats"
        echo "  ./view_render_db.sh users"
        echo "  ./view_render_db.sh transactions all"
        echo "  ./view_render_db.sh query \"SELECT COUNT(*) FROM users;\""
        echo "  ./view_render_db.sh all"
        echo ""
        ;;
esac

