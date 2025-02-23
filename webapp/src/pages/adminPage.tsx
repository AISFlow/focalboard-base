// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import React, {useCallback, useEffect, useState} from 'react'
import {useIntl} from 'react-intl';

import './adminPage.scss'
import client from '../octoClient'
import {IUser} from '../user';
import Sidebar from '../components/sidebar/sidebar';
import BoardTemplateSelector from '../components/boardTemplateSelector/boardTemplateSelector';
import User from '../components/user/user';

import {useAppSelector} from '../store/hooks';
import {getMe} from '../store/users';
import {getClientConfig} from '../store/clientConfig';
import {ClientConfig} from '../config/clientConfig';

const AdminPage = () => {
    const clientConfig = useAppSelector<ClientConfig>(getClientConfig)
    const me = useAppSelector<IUser|null>(getMe)
    const [users, setUsers] = useState<IUser[]>([]);
    const [boardTemplateSelectorOpen, setBoardTemplateSelectorOpen] = useState<boolean>(false)

    const fetchUsers = async () => {
        const result = await client.getTeamUsers();
        setUsers(result)
    };

    useEffect(() => {
        fetchUsers();
    }, []);

    const openBoardTemplateSelector = useCallback(() => {
        setBoardTemplateSelectorOpen(true)
    }, [])
    const closeBoardTemplateSelector = useCallback(() => {
        setBoardTemplateSelectorOpen(false)
    }, [])

    return (
        <div className='AdminPage'>
            <Sidebar
                onBoardTemplateSelectorOpen={openBoardTemplateSelector}
                onBoardTemplateSelectorClose={closeBoardTemplateSelector}
            />
            <div className='panel'>
                {boardTemplateSelectorOpen &&
                    <BoardTemplateSelector onClose={closeBoardTemplateSelector}/>
                }
                <h1 className='ml-3'>Team Users</h1>
                { users.length > 0 && (
                    <h2 className='users-count ml-3'>Users count: {users.length}</h2>
                )}
                <ul className='users-list ml-3'>
                    {users.map((user) => (
                        <li key={user.id} className='user-item'>
                            <User
                                user={user}
                                teammateNameDisplay={clientConfig.teammateNameDisplay}
                                isMe={me && user.id === me.id}
                            />
                        </li>
                    ))}
                </ul>
            </div>
        </div>
    );
};

export default React.memo(AdminPage)
