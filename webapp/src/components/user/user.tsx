import React, {useCallback, useMemo, useState} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';

import {IUser} from '../../user';
import {Utils} from '../../utils';

import Button from '../../widgets/buttons/button';
import DeleteIcon from '../../widgets/icons/delete';
import ConfirmationDialogBox, {ConfirmationDialogBoxProps} from '../confirmationDialogBox'
import mutator from '../../mutator'

import './user.scss'

type Props = {
    user: IUser,
    teammateNameDisplay: string,
    isMe?: boolean | null,
}

const User = (props: Props) => {

    const {user, teammateNameDisplay, isMe} = props
    const intl = useIntl()
    const [showConfirmationDialogBox, setShowConfirmationDialogBox] = useState<boolean>(false)

    const handleDeleteUser = useCallback(() => {
        mutator.deleteUser(user.id)
    }, [user.id])

    const confirmDialogProps: ConfirmationDialogBoxProps = useMemo(() => {
        return {
            heading: intl.formatMessage({id: 'CardDialog.delete-confirmation-dialog-heading', defaultMessage: 'Confirm user delete!'}),
            confirmButtonText: intl.formatMessage({id: 'CardDialog.delete-confirmation-dialog-button-text', defaultMessage: 'Delete'}),
            onConfirm: handleDeleteUser,
            onClose: () => {
                setShowConfirmationDialogBox(false)
            },
        }
    }, [handleDeleteUser])

    return (
        <div className='user'>
            <div className='ml-3'>
                <strong>{Utils.getUserDisplayName(user, teammateNameDisplay)}</strong>
                <strong className='ml-2 text-light'>{`@${user.username}`}</strong>
                {isMe &&
                    <strong className='ml-2 text-light'>{intl.formatMessage({id: 'ShareBoard.userPermissionsYouText', defaultMessage: '(You)'})}</strong>
                }
            </div>
            <Button
                emphasis='grey'
                size='medium'
                title='Delete user'
                icon={ <DeleteIcon/>}
                onClick={() => setShowConfirmationDialogBox(true)}
            >
                <FormattedMessage
                    id='Admin.deleteUser'
                    defaultMessage='Delete user'
                />
            </Button>
            {showConfirmationDialogBox && <ConfirmationDialogBox dialogBox={confirmDialogProps}/>}
        </div>
    );
};

export default React.memo(User)
